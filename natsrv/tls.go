package natsrv

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/nats-io/nats-server/v2/server"
)

const tlsHandshakeTimeout = 2.0

func clientPortTLSConfig(t config.NATSTLSConfig) (*tls.Config, error) {
	tlsConfig, err := server.GenTLSConfig(&server.TLSConfigOpts{
		CertFile: t.CertFile,
		KeyFile:  t.KeyFile,
		CaFile:   t.CAFile,
	})
	if nil != err {
		return nil, err
	}
	if nil != tlsConfig.ClientCAs {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	}
	return tlsConfig, nil
}

func clusterTLSConfig(t config.NATSTLSConfig) (*tls.Config, error) {
	tlsConfig, err := server.GenTLSConfig(&server.TLSConfigOpts{
		CertFile: t.CertFile,
		KeyFile:  t.KeyFile,
		CaFile:   t.CAFile,
		Verify:   "" != t.CAFile,
	})
	if nil != err {
		return nil, err
	}
	tlsConfig.RootCAs = tlsConfig.ClientCAs
	return tlsConfig, nil
}

func inProcessTLSConfig(t config.NATSTLSConfig) (*tls.Config, error) {
	if "" == t.CertFile || ("" == t.CAFile && "" == t.ClientCertFile) {
		return nil, nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}
	if "" != t.ClientCertFile {
		cert, err := tls.LoadX509KeyPair(t.ClientCertFile, t.ClientKeyFile)
		if nil != err {
			return nil, fmt.Errorf("load nats.tls.client_cert_file: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if "" == t.CAFile {
		return tlsConfig, nil
	}
	pool, err := loadCAPool(t.CAFile)
	if nil != err {
		return nil, err
	}
	tlsConfig.VerifyPeerCertificate = verifyChainAgainst(pool)
	return tlsConfig, nil
}

func loadCAPool(file string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(file)
	if nil != err {
		return nil, fmt.Errorf("read nats ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("nats ca file %s holds no PEM certificates", file)
	}
	return pool, nil
}

func verifyChainAgainst(pool *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if 0 == len(rawCerts) {
			return errors.New("nats: server presented no certificate")
		}
		certs := make([]*x509.Certificate, 0, len(rawCerts))
		for _, raw := range rawCerts {
			cert, err := x509.ParseCertificate(raw)
			if nil != err {
				return fmt.Errorf("nats: parse server certificate: %w", err)
			}
			certs = append(certs, cert)
		}
		intermediates := x509.NewCertPool()
		for _, cert := range certs[1:] {
			intermediates.AddCert(cert)
		}
		if _, err := certs[0].Verify(x509.VerifyOptions{Roots: pool, Intermediates: intermediates}); nil != err {
			return fmt.Errorf("nats: verify server certificate: %w", err)
		}
		return nil
	}
}
