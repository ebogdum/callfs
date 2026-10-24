package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSetDocumentValueNilRemovesKey(t *testing.T) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("backend:\n  localfs_tmp_save_max_size: \"64MB\"\n  localfs_ack: \"disk\"\n"), &doc); nil != err {
		t.Fatal(err)
	}
	if err := SetDocumentValue(&doc, "backend.localfs_tmp_save_max_size", nil); nil != err {
		t.Fatal(err)
	}
	out, err := yaml.Marshal(&doc)
	if nil != err {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "localfs_tmp_save_max_size") || !strings.Contains(string(out), "localfs_ack") {
		t.Fatalf("unexpected document:\n%s", out)
	}
}
