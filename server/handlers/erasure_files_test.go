package handlers

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
)

func TestParseErasureHealthFilter(t *testing.T) {
	if got, err := parseErasureHealthFilter(""); nil != err || "" != got {
		t.Fatalf("empty filter = %q, %v", got, err)
	}
	if got, err := parseErasureHealthFilter("reduced"); nil != err || erasure.HealthReduced != got {
		t.Fatalf("reduced filter = %q, %v", got, err)
	}
	for _, raw := range []string{"unknown", "bogus"} {
		if _, err := parseErasureHealthFilter(raw); nil == err {
			t.Fatalf("filter %q accepted", raw)
		}
	}
}

func TestErasureCursorRoundTrip(t *testing.T) {
	encoded := encodeErasureCursor("/data/a.bin")
	if nil == encoded {
		t.Fatal("encoded cursor is nil")
	}
	decoded, err := decodeErasureCursor(*encoded)
	if nil != err || "/data/a.bin" != decoded {
		t.Fatalf("decoded = %q, %v", decoded, err)
	}
	if nil != encodeErasureCursor("") {
		t.Fatal("empty next cursor must encode to null")
	}
	if _, err := decodeErasureCursor("%%%"); nil == err {
		t.Fatal("invalid cursor accepted")
	}
}

func TestErasureFileItemOfUnknownHealth(t *testing.T) {
	info := &metadata.ErasureFileInfo{FilePath: "/a.bin", OriginalSize: 10, DataShards: 4, ParityShards: 2, ShardSize: 3}
	item := erasureFileItemOf(info, erasure.FileHealth{}, false)
	if nil != item.HealthyShards || nil != item.LastCheckedAt {
		t.Fatalf("unknown health must leave verify fields null: %+v", item)
	}
	if erasure.HealthUnknown != item.Health || 6 != item.ShardsTotal {
		t.Fatalf("health=%q shards_total=%d", item.Health, item.ShardsTotal)
	}
}

func TestErasureFileItemOfKnownHealth(t *testing.T) {
	checked := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	info := &metadata.ErasureFileInfo{FilePath: "/a.bin", DataShards: 4, ParityShards: 2}
	item := erasureFileItemOf(info, erasure.FileHealth{Health: erasure.HealthReduced, HealthyShards: 5, CheckedAt: checked}, true)
	if nil == item.HealthyShards || 5 != *item.HealthyShards {
		t.Fatalf("healthy_shards = %v", item.HealthyShards)
	}
	if nil == item.LastCheckedAt || "2026-10-01T12:00:00Z" != *item.LastCheckedAt {
		t.Fatalf("last_checked_at = %v", item.LastCheckedAt)
	}
	if erasure.HealthReduced != item.Health {
		t.Fatalf("health = %q", item.Health)
	}
}

func TestErasureShardKind(t *testing.T) {
	if "data" != erasureShardKind(3, 4) || "parity" != erasureShardKind(4, 4) {
		t.Fatal("shard kind boundary wrong")
	}
}

func TestSettleErasureRepair(t *testing.T) {
	before := erasure.VerifyReport{Shards: []erasure.ShardVerdict{
		{Index: 0, State: erasure.ShardOK},
		{Index: 1, State: erasure.ShardCorrupt},
		{Index: 2, State: erasure.ShardMissing},
	}}
	after := erasure.VerifyReport{Shards: []erasure.ShardVerdict{
		{Index: 0, State: erasure.ShardOK},
		{Index: 1, State: erasure.ShardOK},
		{Index: 2, State: erasure.ShardCorrupt},
	}}
	got := settleErasureRepair(before, after, nil)
	want := erasureRepairResult{
		Repaired: []int{1},
		Failed:   []erasureRepairFailure{{Index: 2, Error: "shard still corrupt"}},
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("settle = %+v, want %+v", got, want)
	}

	failed := settleErasureRepair(before, before, errors.New("boom"))
	wantFailed := []erasureRepairFailure{{Index: 1, Error: "boom"}, {Index: 2, Error: "boom"}}
	if !reflect.DeepEqual(wantFailed, failed.Failed) || 0 != len(failed.Repaired) {
		t.Fatalf("repair error settle = %+v", failed)
	}
}
