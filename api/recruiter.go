package api

type UnreserveRequest struct {
	Addrs []string `json:"addrs"`
}

type UnreserveResponse struct {
	Reserved bool `json:"reserved"`
}

type DeregisterRequest struct {
	Addr string `json:"addr"`
}

type Worker struct {
	Addr     string `json:"addr"`
	NodeName string `json:"node_name"`
}

type RegisterRequest struct {
	Timeout int `json:"timeout"`
	Worker
}

type RegisterResponse struct {
	Directive string `json:"directive"`
}

type RecruitRequest struct {
	Label           string `json:"label"`
	NumberRequested int    `json:"number_requested"`
}

type RecruitResponse struct {
	Workers []Worker `json:"workers"`
}

type RecruiterStatsResponse struct {
	LenFreePool     int `json:"len_free_pool"`
	LenReservedPool int `json:"len_reserved_pool"`
	LenNodePool     int `json:"len_node_pool"`
}
