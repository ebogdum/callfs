package backends

import (
	"context"
	"testing"
)

func TestTrueOverwriteDecision(t *testing.T) {
	if _, err := ParseTrueOverwrite("sometimes"); nil == err {
		t.Fatal("expected an error for a non-boolean value")
	}
	background := context.Background()
	if UseTrueOverwrite(background, false, 0, 1<<30) {
		t.Fatal("default must use the temp save")
	}
	if !UseTrueOverwrite(background, true, 0, 10) {
		t.Fatal("setting on must force a true overwrite")
	}
	if !UseTrueOverwrite(background, false, 1024, 2048) {
		t.Fatal("files bigger than the temp save limit must use a true overwrite")
	}
	if UseTrueOverwrite(background, false, 1024, 1024) {
		t.Fatal("files up to the limit must use the temp save")
	}
	if UseTrueOverwrite(WithTrueOverwrite(background, false), true, 0, 10) {
		t.Fatal("header false must win over the setting")
	}
	if !UseTrueOverwrite(WithTrueOverwrite(background, true), false, 0, 10) {
		t.Fatal("header true must win over the setting")
	}
}
