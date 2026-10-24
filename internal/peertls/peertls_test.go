package peertls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigureRejectsFileWithoutCertificates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); nil != err {
		t.Fatal(err)
	}
	if err := Configure(path, false); nil == err {
		t.Fatal("expected an error for a file without certificates")
	}
}

func TestConfigureSkipVerifyWithoutCA(t *testing.T) {
	if err := Configure("", true); nil != err {
		t.Fatal(err)
	}
	if !ClientConfig().InsecureSkipVerify {
		t.Fatal("skip verify not applied")
	}
	if err := Configure("", false); nil != err {
		t.Fatal(err)
	}
}
