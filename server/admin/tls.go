package admin

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/server/middleware"
)

const (
	protocolHTTP  = "http"
	protocolHTTPS = "https"
	protocolAuto  = "auto"
)

type tlsCertView struct {
	Path          string    `json:"path"`
	Subject       string    `json:"subject"`
	Issuer        string    `json:"issuer"`
	SANs          []string  `json:"sans"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	DaysRemaining int       `json:"days_remaining"`
}

type tlsKeyView struct {
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	WorldReadable bool   `json:"world_readable"`
}

type tlsQUICView struct {
	Enabled bool   `json:"enabled"`
	Addr    string `json:"addr"`
}

type tlsHeadersView struct {
	CSP          string `json:"csp"`
	FrameOptions string `json:"frame_options"`
	COOP         string `json:"coop"`
	COEP         string `json:"coep"`
}

type tlsView struct {
	Enabled            bool           `json:"enabled"`
	ProtocolConfigured string         `json:"protocol_configured"`
	ProtocolEffective  string         `json:"protocol_effective"`
	Fallback           bool           `json:"fallback"`
	Cert               *tlsCertView   `json:"cert"`
	Key                *tlsKeyView    `json:"key"`
	QUIC               tlsQUICView    `json:"quic"`
	HSTS               bool           `json:"hsts"`
	Headers            tlsHeadersView `json:"headers"`
}

type headerCapture struct {
	header http.Header
}

func (c *headerCapture) Header() http.Header {
	return c.header
}

func (c *headerCapture) Write(payload []byte) (int, error) {
	return len(payload), nil
}

func (c *headerCapture) WriteHeader(int) {}

func (a *API) TLS(w http.ResponseWriter, _ *http.Request) {
	logger := a.logger()
	cfg := a.deps.Config
	if nil == cfg {
		handlers.SendErrorResponse(w, logger, errors.New("configuration is not loaded"), http.StatusInternalServerError)
		return
	}
	view, err := buildTLSView(cfg, time.Now())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	handlers.SendJSONResponse(w, view)
}

func buildTLSView(cfg *config.AppConfig, now time.Time) (tlsView, error) {
	effective, fallback := effectiveProtocol(cfg)
	headers := securityHeaders(protocolHTTPS == effective)
	view := tlsView{
		Enabled:            protocolHTTPS == effective,
		ProtocolConfigured: cfg.Server.Protocol,
		ProtocolEffective:  effective,
		Fallback:           fallback,
		QUIC:               tlsQUICView{Enabled: cfg.Server.EnableQUIC, Addr: cfg.Server.QUICListenAddr},
		HSTS:               "" != headers.Get("Strict-Transport-Security"),
		Headers: tlsHeadersView{
			CSP:          headers.Get("Content-Security-Policy"),
			FrameOptions: headers.Get("X-Frame-Options"),
			COOP:         headers.Get("Cross-Origin-Opener-Policy"),
			COEP:         headers.Get("Cross-Origin-Embedder-Policy"),
		},
	}
	if protocolHTTP == effective {
		return view, nil
	}
	if "" != cfg.Server.CertFile {
		cert, err := loadLeafCertificate(cfg.Server.CertFile)
		if nil != err {
			return tlsView{}, err
		}
		view.Cert = &tlsCertView{
			Path:          cfg.Server.CertFile,
			Subject:       cert.Subject.String(),
			Issuer:        cert.Issuer.String(),
			SANs:          subjectAlternatives(cert),
			NotBefore:     cert.NotBefore,
			NotAfter:      cert.NotAfter,
			DaysRemaining: int(cert.NotAfter.Sub(now).Hours() / 24),
		}
	}
	if "" != cfg.Server.KeyFile {
		perm, err := keyPermissions(cfg.Server.KeyFile)
		if nil != err {
			return tlsView{}, err
		}
		view.Key = &tlsKeyView{
			Path:          cfg.Server.KeyFile,
			Mode:          fmt.Sprintf("%04o", uint32(perm)),
			WorldReadable: 0 != perm&0o004,
		}
	}
	return view, nil
}

func effectiveProtocol(cfg *config.AppConfig) (string, bool) {
	switch strings.ToLower(cfg.Server.Protocol) {
	case protocolHTTP:
		return protocolHTTP, false
	case protocolAuto:
		if "" == cfg.Server.CertFile || "" == cfg.Server.KeyFile {
			return protocolHTTP, true
		}
		return protocolHTTPS, false
	}
	return protocolHTTPS, false
}

func loadLeafCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if nil != err {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}
	block, _ := pem.Decode(data)
	if nil == block || "CERTIFICATE" != block.Type {
		return nil, fmt.Errorf("certificate file %s holds no PEM certificate", path)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if nil != err {
		return nil, fmt.Errorf("failed to parse certificate file: %w", err)
	}
	return cert, nil
}

func keyPermissions(path string) (fs.FileMode, error) {
	info, err := os.Stat(path)
	if nil != err {
		return 0, fmt.Errorf("failed to stat key file: %w", err)
	}
	return info.Mode().Perm(), nil
}

func subjectAlternatives(cert *x509.Certificate) []string {
	sans := make([]string, 0, len(cert.DNSNames)+len(cert.IPAddresses))
	sans = append(sans, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		sans = append(sans, ip.String())
	}
	return sans
}

func securityHeaders(secure bool) http.Header {
	capture := &headerCapture{header: make(http.Header)}
	req := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: "/"}, Header: make(http.Header)}
	if secure {
		req.TLS = &tls.ConnectionState{}
	}
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	middleware.V1SecurityHeaders()(next).ServeHTTP(capture, req)
	return capture.header
}
