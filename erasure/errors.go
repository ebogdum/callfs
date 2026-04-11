package erasure

import "errors"

// Error codes 3050-3069: Erasure coding errors
var (
	ErrInsufficientShards = errors.New("erasure: insufficient shards for reconstruction (code 3050)")
	ErrShardCorrupted     = errors.New("erasure: shard checksum mismatch (code 3051)")
	ErrInvalidProfile     = errors.New("erasure: invalid erasure profile parameters (code 3054)")
	ErrShardNotFound      = errors.New("erasure: shard not found on this node (code 3055)")
)
