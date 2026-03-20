package erasure

// ErasureProfile defines the Reed-Solomon encoding parameters for a file.
type ErasureProfile struct {
	DataShards   int   `json:"data_shards"`
	ParityShards int   `json:"parity_shards"`
	ShardSize    int64 `json:"shard_size"`
}

// ShardInfo describes a single shard's storage location.
type ShardInfo struct {
	Index       int    `json:"index"`
	InstanceID  string `json:"instance_id"`
	BackendType string `json:"backend_type"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
}

// ErasureFileInfo holds the complete erasure coding metadata for a stored file.
type ErasureFileInfo struct {
	FilePath     string         `json:"file_path"`
	OriginalSize int64          `json:"original_size"`
	Profile      ErasureProfile `json:"profile"`
	Shards       []ShardInfo    `json:"shards"`
}

// ChunkManifest is returned to clients for parallel multi-server download.
type ChunkManifest struct {
	Path           string          `json:"path"`
	OriginalSize   int64           `json:"original_size"`
	ErasureProfile ErasureProfile  `json:"erasure_profile"`
	Shards         []ShardEndpoint `json:"shards"`
}

// ShardEndpoint maps a shard index to its direct download URL.
type ShardEndpoint struct {
	Index    int    `json:"index"`
	Endpoint string `json:"endpoint"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

// StoreOptions allows clients to override server defaults for erasure coding.
type StoreOptions struct {
	DataShards   int      // 0 = use server default
	ParityShards int      // 0 = use server default
	Instances    []string // nil = use placement strategy across all available
}
