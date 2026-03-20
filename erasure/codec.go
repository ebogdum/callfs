package erasure

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/klauspost/reedsolomon"
)

// Codec wraps Reed-Solomon encoding and decoding.
type Codec struct{}

// NewCodec creates a new erasure Codec.
func NewCodec() *Codec {
	return &Codec{}
}

// Encode splits data into dataShards pieces and computes parityShards parity blocks.
// Returns dataShards+parityShards slices. Each shard has equal length (data is padded).
func (c *Codec) Encode(data []byte, profile ErasureProfile) ([][]byte, error) {
	if profile.DataShards < 1 || profile.ParityShards < 1 {
		return nil, ErrInvalidProfile
	}

	enc, err := reedsolomon.New(profile.DataShards, profile.ParityShards)
	if err != nil {
		return nil, fmt.Errorf("erasure: failed to create encoder: %w", err)
	}

	shards, err := enc.Split(data)
	if err != nil {
		return nil, fmt.Errorf("erasure: failed to split data: %w", err)
	}

	if err := enc.Encode(shards); err != nil {
		return nil, fmt.Errorf("erasure: failed to encode parity: %w", err)
	}

	return shards, nil
}

// Decode reconstructs the original data from shards. Nil entries are treated as missing.
// At least dataShards non-nil shards are required.
func (c *Codec) Decode(shards [][]byte, profile ErasureProfile, originalSize int64) ([]byte, error) {
	if profile.DataShards < 1 || profile.ParityShards < 1 {
		return nil, ErrInvalidProfile
	}

	enc, err := reedsolomon.New(profile.DataShards, profile.ParityShards)
	if err != nil {
		return nil, fmt.Errorf("erasure: failed to create decoder: %w", err)
	}

	if err := enc.Reconstruct(shards); err != nil {
		return nil, fmt.Errorf("erasure: reconstruction failed: %w", err)
	}

	ok, err := enc.Verify(shards)
	if err != nil {
		return nil, fmt.Errorf("erasure: verification failed: %w", err)
	}
	if !ok {
		return nil, ErrShardCorrupted
	}

	// Join data shards and trim to original size
	buf := make([]byte, 0, originalSize)
	for i := 0; i < profile.DataShards; i++ {
		buf = append(buf, shards[i]...)
	}

	if int64(len(buf)) < originalSize {
		return nil, ErrInsufficientShards
	}

	return buf[:originalSize], nil
}

// ShardChecksum returns the SHA-256 hex digest of a shard.
func ShardChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
