// +build zeek

package zqd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/brimsec/zq/pkg/test"
	"github.com/brimsec/zq/zqd"
	"github.com/brimsec/zq/zqd/api"
	"github.com/brimsec/zq/zqd/packet"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestPacketPost(t *testing.T) {
	suite.Run(t, new(PacketPostSuite))
}

type PacketPostSuite struct {
	suite.Suite
	root     string
	space    string
	pcapfile string
	payloads []interface{}
}

func (s *PacketPostSuite) TestCount() {
	expected := `
#0:record[count:uint64]
0:[2;]`
	res := execSearch(s.T(), s.root, s.space, "count()")
	s.Equal(test.Trim(expected), res)
}

func (s *PacketPostSuite) TestNoPacketFilter() {
	expected := ""
	res := execSearch(s.T(), s.root, s.space, "_path = packet_filter | count()")
	s.Equal(expected, res)
}

func (s *PacketPostSuite) TestSpaceInfo() {
	u := fmt.Sprintf("http://localhost:9867/space/%s", s.space)
	res := httpSuccess(s.T(), zqd.NewHandler(s.root), "GET", u, nil)
	var info api.SpaceInfo
	err := json.NewDecoder(res).Decode(&info)
	s.NoError(err)
	s.Equal(s.pcapfile, info.PacketPath)
	s.True(info.PacketSupport)
}

func (s *PacketPostSuite) TestWritesIndexFile() {
	stat, err := os.Stat(filepath.Join(s.root, s.space, packet.IndexFile))
	s.NoError(err)
	s.NotNil(stat)
}

func (s *PacketPostSuite) TestStatus() {
	info, err := os.Stat(s.pcapfile)
	s.NoError(err)
	s.Len(s.payloads, 3)
	status := s.payloads[1].(*api.PacketPostStatus)
	s.Equal(status.Type, "PacketPostStatus")
	s.Equal(status.PacketSize, info.Size())
	s.Equal(status.PacketReadSize, info.Size())
}

func (s *PacketPostSuite) SetupTest() {
	s.space = "test"
	dir, err := ioutil.TempDir("", "PacketPostTest")
	s.NoError(err)
	s.root = dir
	s.pcapfile = filepath.Join(".", "testdata/test.pcap")
	s.payloads = createSpaceWithPcap(s.T(), s.root, s.space, s.pcapfile)
}

func (s *PacketPostSuite) TearDownTest() {
	os.RemoveAll(s.root)
}

func createSpaceWithPcap(t *testing.T, root, spaceName, pcapfile string) []interface{} {
	createSpace(t, root, spaceName, "")
	req := api.PacketPostRequest{filepath.Join(".", pcapfile)}
	u := fmt.Sprintf("http://localhost:9867/space/%s/packet", spaceName)
	body := httpSuccess(t, zqd.NewHandler(root), "POST", u, req)
	scanner := api.NewJSONPipeScanner(body)
	_, cancel := context.WithCancel(context.Background())
	stream := api.NewStream(scanner, cancel)
	var taskEnd api.TaskEnd
	var payloads []interface{}
	for {
		i, err := stream.Next()
		require.NoError(t, err)
		if i == nil {
			break
		}
		payloads = append(payloads, i)
		if end, ok := i.(api.TaskEnd); ok {
			taskEnd = end
		}
	}
	require.Nil(t, taskEnd.Error)
	return payloads
}
