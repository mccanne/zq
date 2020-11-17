package api

type RecruitRequest struct {
	SearchRequest
	ChunkPaths []string `json:"chunk_paths"`
	DataPath   string   `json:"data_path"`
	Label      string   `json:"label"`
}

type RecruitResponse struct {
	Version string `json:"version"`
}
