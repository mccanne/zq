package apiserver

import (
	"context"
	"fmt"
	"path"
	"sync"

	"github.com/brimsec/zq/api"
	"github.com/brimsec/zq/pkg/iosrc"
	"github.com/brimsec/zq/pkg/nano"
	"github.com/brimsec/zq/ppl/zqd/apiserver/oldconfig"
	"github.com/brimsec/zq/ppl/zqd/apiserver/space"
	"github.com/brimsec/zq/ppl/zqd/pcapstorage"
	"github.com/brimsec/zq/ppl/zqd/storage"
	"github.com/brimsec/zq/ppl/zqd/storage/archivestore"
	"github.com/brimsec/zq/ppl/zqd/storage/filestore"
	"github.com/brimsec/zq/zqe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

type Manager struct {
	alphaFileMigrator *filestore.Migrator
	compactor         *compactor
	db                *FileDB
	logger            *zap.Logger
	rootPath          iosrc.URI

	// We keep instances of any loaded filestore because the seek indexes
	// we create for them are not persisted to disk. We are unlikely to
	// implement persistence, since we intend to use archive based
	// storage by default at some point in the future.
	filestores   map[api.SpaceID]*filestore.Storage
	filestoresMu sync.Mutex

	created prometheus.Counter
	deleted prometheus.Counter
}

func NewManager(ctx context.Context, logger *zap.Logger, registerer prometheus.Registerer, root iosrc.URI) (*Manager, error) {
	db, err := prepareFileDB(ctx, logger, root)
	if err != nil {
		return nil, err
	}

	factory := promauto.With(registerer)
	m := &Manager{
		logger:   logger,
		rootPath: root,
		db:       db,

		alphaFileMigrator: filestore.NewMigrator(ctx),
		filestores:        make(map[api.SpaceID]*filestore.Storage),

		created: factory.NewCounter(prometheus.CounterOpts{
			Name: "spaces_created_total",
			Help: "Number of spaces created.",
		}),
		deleted: factory.NewCounter(prometheus.CounterOpts{
			Name: "spaces_deleted_total",
			Help: "Number of spaces deleted.",
		}),
	}
	m.compactor = newCompactor(m)
	m.spawnAlphaMigrations(ctx)
	return m, nil
}

func (m *Manager) Shutdown() {
	m.compactor.close()
}

const dbname = "zqd.json"

func prepareFileDB(ctx context.Context, logger *zap.Logger, root iosrc.URI) (*FileDB, error) {
	dburi := root.AppendPath(dbname)
	exists, err := iosrc.Exists(ctx, dburi)
	if err != nil {
		return nil, err
	}
	if exists {
		return OpenFileDB(ctx, dburi)
	}

	// If the dbfile doesn't exist, we check if we need to migrate the older
	// per-space config files into a new dbfile.
	configs, err := oldconfig.LoadConfigs(ctx, logger, root)
	if err != nil {
		return nil, err
	}
	var rows []SpaceRow
	for id, config := range configs {
		datauri := config.DataURI
		if datauri.IsZero() {
			datauri = root.AppendPath(string(id))
		}
		rows = append(rows, SpaceRow{
			ID:      id,
			Name:    config.Name,
			DataURI: datauri,
			Storage: config.Storage,
		})
		for _, subcfg := range config.Subspaces {
			openopts := subcfg.OpenOptions
			rows = append(rows, SpaceRow{
				ID:       subcfg.ID,
				ParentID: id,
				Name:     subcfg.Name,
				DataURI:  datauri,
				Storage: api.StorageConfig{
					Kind: api.ArchiveStore,
					Archive: &api.ArchiveConfig{
						OpenOptions: &openopts,
					},
				},
			})
		}
	}
	return CreateFileDB(ctx, dburi, rows)
}

func (m *Manager) spawnAlphaMigrations(ctx context.Context) {
	rows, err := m.db.ListSpaces(ctx)
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.Storage.Kind != api.FileStore {
			continue
		}
		store, err := m.getStorage(ctx, row.ID, row.DataURI, row.Storage)
		if err != nil {
			continue
		}
		m.alphaFileMigrator.Add(store.(*filestore.Storage))
	}
}

func (m *Manager) CreateSpace(ctx context.Context, req api.SpacePostRequest) (api.Space, error) {
	if req.Name == "" && req.DataPath == "" {
		return api.Space{}, zqe.E(zqe.Invalid, "must supply non-empty name or dataPath")
	}
	if !space.ValidSpaceName(req.Name) {
		return api.Space{}, zqe.E(zqe.Invalid, "name may not contain '/' or non-printable characters")
	}
	id := space.NewSpaceID()
	var datapath iosrc.URI
	if req.DataPath == "" {
		datapath = m.rootPath.AppendPath(string(id))
	} else {
		var err error
		datapath, err = iosrc.ParseURI(req.DataPath)
		if err != nil {
			return api.Space{}, err
		}
	}
	if err := iosrc.MkdirAll(datapath, 0777); err != nil {
		return api.Space{}, err
	}
	// If name is not set then derive name from DataPath, removing and
	// replacing invalid characters.
	var retryNameConflict bool
	if req.Name == "" {
		retryNameConflict = true
		req.Name = space.SafeName(path.Base(datapath.Path))
	}

	var storecfg api.StorageConfig
	if req.Storage != nil {
		storecfg = *req.Storage
	}
	if storecfg.Kind == api.UnknownStore {
		storecfg.Kind = api.FileStore
	}
	if storecfg.Kind == api.FileStore && m.rootPath.Scheme != "file" {
		return api.Space{}, zqe.E(zqe.Invalid, "cannot create file storage space on non-file backed data path")
	}

	row := SpaceRow{
		ID:      id,
		Name:    req.Name,
		DataURI: datapath,
		Storage: storecfg,
	}

	name := row.Name
	for i := 1; ; i++ {
		err := m.db.CreateSpace(ctx, row)
		if err == nil {
			break
		}
		if retryNameConflict && zqe.IsConflict(err) {
			row.Name = fmt.Sprintf("%s_%d", name, i)
			continue
		}
		return api.Space{}, err
	}

	si := rowToSpace(row)
	m.created.Inc()
	return si, nil
}

func (m *Manager) CreateSubspace(ctx context.Context, parentID api.SpaceID, req api.SubspacePostRequest) (api.Space, error) {
	if req.Name == "" {
		return api.Space{}, zqe.E(zqe.Invalid, "cannot set name to an empty string")
	}
	if !space.ValidSpaceName(req.Name) {
		return api.Space{}, zqe.E(zqe.Invalid, "name may not contain '/' or non-printable characters")
	}
	parent, err := m.db.GetSpace(ctx, parentID)
	if err != nil {
		return api.Space{}, err
	}
	if parent.Storage.Kind != api.ArchiveStore {
		return api.Space{}, zqe.E(zqe.Invalid, "space does not support creating subspaces")
	}
	row := SpaceRow{
		ID:       space.NewSpaceID(),
		ParentID: parentID,
		Name:     req.Name,
		DataURI:  parent.DataURI,
		Storage: api.StorageConfig{
			Kind: api.ArchiveStore,
			Archive: &api.ArchiveConfig{
				OpenOptions: &req.OpenOptions,
			},
		},
	}
	if err := m.db.CreateSubspace(ctx, row); err != nil {
		return api.Space{}, err
	}
	m.created.Inc()
	return rowToSpace(row), nil
}

func (m *Manager) GetStorage(ctx context.Context, id api.SpaceID) (storage.Storage, error) {
	sr, err := m.db.GetSpace(ctx, id)
	if err != nil {
		return nil, err
	}
	return m.getStorage(ctx, id, sr.DataURI, sr.Storage)
}

func (m *Manager) GetPcapStorage(ctx context.Context, id api.SpaceID) (*pcapstorage.Store, error) {
	sr, err := m.db.GetSpace(ctx, id)
	if err != nil {
		return nil, err
	}
	return m.getPcapStorage(ctx, sr.DataURI)
}

type writeNotifier struct {
	c  *compactor
	id api.SpaceID
}

func (n writeNotifier) WriteNotify() {
	n.c.WriteComplete(n.id)
}

func (m *Manager) getStorage(ctx context.Context, id api.SpaceID, daturi iosrc.URI, cfg api.StorageConfig) (storage.Storage, error) {
	switch cfg.Kind {
	case api.FileStore:
		m.filestoresMu.Lock()
		defer m.filestoresMu.Unlock()
		st, ok := m.filestores[id]
		if !ok {
			var err error
			st, err = filestore.Load(daturi, m.logger)
			if err != nil {
				return nil, err
			}
			m.filestores[id] = st
		}
		return st, nil
	case api.ArchiveStore:
		wn := &writeNotifier{c: m.compactor, id: id}
		return archivestore.Load(ctx, daturi, wn, cfg.Archive)
	default:
		return nil, zqe.E(zqe.Invalid, "unknown storage kind: %s", cfg.Kind)
	}
}

func (m *Manager) getPcapStorage(ctx context.Context, datauri iosrc.URI) (*pcapstorage.Store, error) {
	p, err := pcapstorage.Load(ctx, datauri)
	if err != nil {
		if zqe.IsNotFound(err) {
			return pcapstorage.New(datauri), nil
		}
		return nil, err
	}
	return p, err
}

func rowToSpace(row SpaceRow) api.Space {
	return api.Space{
		ID:          row.ID,
		DataPath:    row.DataURI,
		Name:        row.Name,
		ParentID:    row.ParentID,
		StorageKind: row.Storage.Kind,
	}
}

func (m *Manager) rowToSpaceInfo(ctx context.Context, sr SpaceRow) (api.SpaceInfo, error) {
	spaceInfo := api.SpaceInfo{Space: rowToSpace(sr)}

	store, err := m.getStorage(ctx, sr.ID, sr.DataURI, sr.Storage)
	if err != nil {
		return api.SpaceInfo{}, err
	}
	sum, err := store.Summary(ctx)
	if err != nil {
		return api.SpaceInfo{}, err
	}
	var span *nano.Span
	if sum.Span.Dur > 0 {
		span = &sum.Span
	}
	spaceInfo.Span = span
	spaceInfo.StorageKind = sum.Kind
	spaceInfo.Size = sum.DataBytes

	pcapstore, err := m.getPcapStorage(ctx, sr.DataURI)
	if err != nil {
		return api.SpaceInfo{}, err
	}
	if !pcapstore.Empty() {
		pcapinfo, err := pcapstore.Info(ctx)
		if err != nil {
			if !zqe.IsNotFound(err) {
				return api.SpaceInfo{}, err
			}
		} else {
			spaceInfo.PcapSize = pcapinfo.PcapSize
			spaceInfo.PcapSupport = true
			spaceInfo.PcapPath = pcapinfo.PcapURI
			if span == nil {
				span = &pcapinfo.Span
			} else {
				union := span.Union(pcapinfo.Span)
				span = &union
			}
		}
	}

	return spaceInfo, nil
}

func (m *Manager) GetSpace(ctx context.Context, id api.SpaceID) (api.SpaceInfo, error) {
	sr, err := m.db.GetSpace(ctx, id)
	if err != nil {
		return api.SpaceInfo{}, err
	}
	return m.rowToSpaceInfo(ctx, sr)
}

func (m *Manager) ListSpaces(ctx context.Context) ([]api.Space, error) {
	rows, err := m.db.ListSpaces(ctx)
	if err != nil {
		return nil, err
	}
	res := make([]api.Space, 0, len(rows))
	for _, row := range rows {
		res = append(res, rowToSpace(row))
	}
	return res, nil
}

func (m *Manager) UpdateSpaceName(ctx context.Context, id api.SpaceID, name string) error {
	if name == "" {
		return zqe.E(zqe.Invalid, "cannot set name to an empty string")
	}
	if !space.ValidSpaceName(name) {
		return zqe.E(zqe.Invalid, "name may not contain '/' or non-printable characters")
	}
	return m.db.UpdateSpaceName(ctx, id, name)
}

func (m *Manager) DeleteSpace(ctx context.Context, id api.SpaceID) error {
	sr, err := m.db.GetSpace(ctx, id)
	if err != nil {
		return err
	}
	if err := m.db.DeleteSpace(ctx, id); err != nil {
		return err
	}
	if sr.Storage.Kind == api.FileStore {
		m.filestoresMu.Lock()
		delete(m.filestores, id)
		m.filestoresMu.Unlock()
	}
	if err := iosrc.RemoveAll(ctx, sr.DataURI); err != nil {
		return err
	}
	m.deleted.Inc()
	return nil
}
