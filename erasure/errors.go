package erasure

import "errors"

// Error codes 3050-3069: Erasure coding errors
var (
	ErrInsufficientShards = errors.New("erasure: insufficient shards for reconstruction (code 3050)")
	ErrShardCorrupted     = errors.New("erasure: shard checksum mismatch (code 3051)")
	ErrInsufficientNodes  = errors.New("erasure: not enough nodes for shard placement (code 3052)")
	ErrFileTooSmall       = errors.New("erasure: file below minimum size for erasure coding (code 3053)")
	ErrInvalidProfile     = errors.New("erasure: invalid erasure profile parameters (code 3054)")
	ErrShardNotFound      = errors.New("erasure: shard not found on this node (code 3055)")
)
