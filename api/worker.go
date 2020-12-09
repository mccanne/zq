package api

type WorkerChunkRequest struct {
	SearchRequest
	ChunkPaths []string `json:"chunk_paths"`
	DataPath   string   `json:"data_path"`
}

type WorkerRootRequest struct {
	SearchRequest
	MaxWorkers int `json:"max_workers"`
}
