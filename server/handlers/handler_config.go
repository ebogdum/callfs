package handlers

// HandlerConfig specifies which handlers to use
type HandlerConfig struct {
	EnableCrossServerSupport bool `yaml:"enable_cross_server_support" json:"enable_cross_server_support"`
}

// CrossServerStatus represents the status of cross-server functionality
type CrossServerStatus struct {
	Enabled         bool              `json:"enabled"`
	CurrentInstance string            `json:"current_instance"`
	PeerInstances   map[string]string `json:"peer_instances"`
	SupportedOps    []string          `json:"supported_operations"`
}
