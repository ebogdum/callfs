package erasure

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncodeDecodeRoundtrip(t *testing.T) {
	codec := NewCodec()
	profile := ErasureProfile{DataShards: 4, ParityShards: 2}

	original := make([]byte, 1024*100) // 100KB
	if _, err := rand.Read(original); err != nil {
		t.Fatal(err)
	}

	shards, err := codec.Encode(original, profile)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(shards) != 6 {
		t.Fatalf("expected 6 shards, got %d", len(shards))
	}

	decoded, err := codec.Decode(shards, profile, int64(len(original)))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if !bytes.Equal(original, decoded) {
		t.Fatal("decoded data does not match original")
	}
}

func TestDecodeDegraded(t *testing.T) {
	codec := NewCodec()
	profile := ErasureProfile{DataShards: 4, ParityShards: 2}

	original := make([]byte, 1024*50)
	if _, err := rand.Read(original); err != nil {
		t.Fatal(err)
	}

	shards, err := codec.Encode(original, profile)
	if err != nil {
		t.Fatal(err)
	}

	// Remove up to parityShards (2) shards
	shards[1] = nil
	shards[4] = nil

	decoded, err := codec.Decode(shards, profile, int64(len(original)))
	if err != nil {
		t.Fatalf("Degraded decode failed: %v", err)
	}

	if !bytes.Equal(original, decoded) {
		t.Fatal("degraded decoded data does not match original")
	}
}

func TestDecodeFailsTooManyMissing(t *testing.T) {
	codec := NewCodec()
	profile := ErasureProfile{DataShards: 4, ParityShards: 2}

	original := make([]byte, 1024*50)
	if _, err := rand.Read(original); err != nil {
		t.Fatal(err)
	}

	shards, err := codec.Encode(original, profile)
	if err != nil {
		t.Fatal(err)
	}

	// Remove 3 shards (more than parity count of 2)
	shards[0] = nil
	shards[2] = nil
	shards[4] = nil

	_, err = codec.Decode(shards, profile, int64(len(original)))
	if err == nil {
		t.Fatal("expected decode to fail with too many missing shards")
	}
}

func TestShardChecksum(t *testing.T) {
	data := []byte("hello world")
	checksum := ShardChecksum(data)

	if len(checksum) != 64 {
		t.Fatalf("expected 64 char hex digest, got %d chars", len(checksum))
	}

	// Same data should produce same checksum
	if ShardChecksum(data) != checksum {
		t.Fatal("checksum not deterministic")
	}

	// Different data should produce different checksum
	if ShardChecksum([]byte("hello world!")) == checksum {
		t.Fatal("different data produced same checksum")
	}
}

func TestEncodeInvalidProfile(t *testing.T) {
	codec := NewCodec()

	_, err := codec.Encode([]byte("test"), ErasureProfile{DataShards: 0, ParityShards: 2})
	if err != ErrInvalidProfile {
		t.Fatalf("expected ErrInvalidProfile, got %v", err)
	}

	_, err = codec.Encode([]byte("test"), ErasureProfile{DataShards: 4, ParityShards: 0})
	if err != ErrInvalidProfile {
		t.Fatalf("expected ErrInvalidProfile, got %v", err)
	}
}

func TestEncodeSmallData(t *testing.T) {
	codec := NewCodec()
	profile := ErasureProfile{DataShards: 4, ParityShards: 2}

	// Very small data (less than dataShards bytes)
	original := []byte("hi")
	shards, err := codec.Encode(original, profile)
	if err != nil {
		t.Fatalf("Encode failed for small data: %v", err)
	}

	decoded, err := codec.Decode(shards, profile, int64(len(original)))
	if err != nil {
		t.Fatalf("Decode failed for small data: %v", err)
	}

	if !bytes.Equal(original, decoded) {
		t.Fatal("decoded small data does not match original")
	}
}
