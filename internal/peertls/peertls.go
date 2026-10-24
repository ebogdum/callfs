package peertls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync/atomic"
)

var current atomic.Pointer[tls.Config]

func Configure(caFile string, skipVerify bool) error {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: skipVerify}
	if "" != caFile {
		pem, err := os.ReadFile(caFile)
		if nil != err {
			return fmt.Errorf("read peer CA file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if nil != err || nil == pool {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("peer CA file %s holds no PEM certificates", caFile)
		}
		cfg.RootCAs = pool
	}
	current.Store(cfg)
	return nil
}

func ClientConfig() *tls.Config {
	if cfg := current.Load(); nil != cfg {
		return cfg.Clone()
	}
	return &tls.Config{MinVersion: tls.VersionTLS12}
}
