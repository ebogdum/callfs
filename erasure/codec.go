package erasure

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/klauspost/reedsolomon"
)

type encoderKey struct {
	dataShards   int
	parityShards int
}

type Codec struct {
	encoders sync.Map
}

func NewCodec() *Codec {
	return &Codec{}
}

func (c *Codec) encoder(dataShards, parityShards int) (reedsolomon.Encoder, error) {
	key := encoderKey{dataShards: dataShards, parityShards: parityShards}
	if cached, ok := c.encoders.Load(key); ok {
		return cached.(reedsolomon.Encoder), nil
	}
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, err
	}
	actual, _ := c.encoders.LoadOrStore(key, enc)
	return actual.(reedsolomon.Encoder), nil
}

func (c *Codec) Encode(data []byte, profile ErasureProfile) ([][]byte, error) {
	if profile.DataShards < 1 || profile.ParityShards < 1 {
		return nil, ErrInvalidProfile
	}

	enc, err := c.encoder(profile.DataShards, profile.ParityShards)
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

func (c *Codec) Decode(shards [][]byte, profile ErasureProfile, originalSize int64) ([]byte, error) {
	if profile.DataShards < 1 || profile.ParityShards < 1 {
		return nil, ErrInvalidProfile
	}
	if originalSize < 0 {
		return nil, fmt.Errorf("erasure: invalid original size %d", originalSize)
	}

	enc, err := c.encoder(profile.DataShards, profile.ParityShards)
	if err != nil {
		return nil, fmt.Errorf("erasure: failed to create decoder: %w", err)
	}

	if err := enc.ReconstructData(shards); err != nil {
		return nil, fmt.Errorf("erasure: reconstruction failed: %w", err)
	}

	out := make([]byte, originalSize)
	var written int64
	for i := 0; i < profile.DataShards && written < originalSize; i++ {
		written += int64(copy(out[written:], shards[i]))
	}
	if written < originalSize {
		return nil, ErrInsufficientShards
	}

	return out, nil
}

func ShardChecksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
