package natsrv

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nkeys"
)

type testTLSFiles struct {
	caFile   string
	certFile string
	keyFile  string
}

func writeTestTLSFiles(t *testing.T) testTLSFiles {
	t.Helper()
	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if nil != err {
		t.Fatalf("generate ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "callfs-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if nil != err {
		t.Fatalf("create ca certificate: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if nil != err {
		t.Fatalf("parse ca certificate: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if nil != err {
		t.Fatalf("generate leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "callfs-test-node"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if nil != err {
		t.Fatalf("create leaf certificate: %v", err)
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if nil != err {
		t.Fatalf("marshal leaf key: %v", err)
	}

	files := testTLSFiles{
		caFile:   filepath.Join(dir, "ca.pem"),
		certFile: filepath.Join(dir, "node.pem"),
		keyFile:  filepath.Join(dir, "node-key.pem"),
	}
	writePEM(t, files.caFile, "CERTIFICATE", caDER)
	writePEM(t, files.certFile, "CERTIFICATE", leafDER)
	writePEM(t, files.keyFile, "EC PRIVATE KEY", leafKeyDER)
	return files
}

func writePEM(t *testing.T, path string, blockType string, der []byte) {
	t.Helper()
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, data, 0o600); nil != err {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestBuildOptionsTLS(t *testing.T) {
	files := writeTestTLSFiles(t)
	cfg := testNATSConfig(t)
	cfg.Routes = []string{"nats-route://127.0.0.1:16222"}
	cfg.TLS = config.NATSTLSConfig{
		CertFile:       files.certFile,
		KeyFile:        files.keyFile,
		CAFile:         files.caFile,
		ClientCertFile: files.certFile,
		ClientKeyFile:  files.keyFile,
	}

	opts, err := BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	if !opts.TLS || nil == opts.TLSConfig {
		t.Fatal("client port TLS not configured")
	}
	if tls.VerifyClientCertIfGiven != opts.TLSConfig.ClientAuth {
		t.Errorf("client port ClientAuth = %v, want VerifyClientCertIfGiven", opts.TLSConfig.ClientAuth)
	}
	if nil == opts.Cluster.TLSConfig {
		t.Fatal("cluster TLS not configured")
	}
	if tls.RequireAndVerifyClientCert != opts.Cluster.TLSConfig.ClientAuth {
		t.Errorf("cluster ClientAuth = %v, want RequireAndVerifyClientCert", opts.Cluster.TLSConfig.ClientAuth)
	}
	if nil == opts.Cluster.TLSConfig.RootCAs || nil == opts.Cluster.TLSConfig.ClientCAs {
		t.Error("cluster TLS does not trust ca_file")
	}
	if 1 != len(opts.Cluster.TLSConfig.Certificates) {
		t.Errorf("cluster certificates = %d, want 1", len(opts.Cluster.TLSConfig.Certificates))
	}

	inProcess, err := inProcessTLSConfig(cfg.TLS)
	if nil != err {
		t.Fatalf("in-process tls: %v", err)
	}
	if nil == inProcess || 1 != len(inProcess.Certificates) || nil == inProcess.VerifyPeerCertificate {
		t.Fatal("in-process TLS must present the client cert and verify against ca_file")
	}
	leaf, err := tls.LoadX509KeyPair(files.certFile, files.keyFile)
	if nil != err {
		t.Fatalf("load leaf: %v", err)
	}
	if err := inProcess.VerifyPeerCertificate(leaf.Certificate, nil); nil != err {
		t.Errorf("leaf signed by ca_file rejected: %v", err)
	}
	other := writeTestTLSFiles(t)
	foreign, err := tls.LoadX509KeyPair(other.certFile, other.keyFile)
	if nil != err {
		t.Fatalf("load foreign leaf: %v", err)
	}
	if err := inProcess.VerifyPeerCertificate(foreign.Certificate, nil); nil == err {
		t.Error("leaf signed by another CA accepted")
	}
}

func TestBuildOptionsWithoutTLS(t *testing.T) {
	cfg := testNATSConfig(t)
	opts, err := BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	if opts.TLS || nil != opts.TLSConfig || nil != opts.Cluster.TLSConfig {
		t.Error("TLS configured without nats.tls.cert_file")
	}
	inProcess, err := inProcessTLSConfig(cfg.TLS)
	if nil != err || nil != inProcess {
		t.Errorf("in-process TLS = %v, %v, want nil, nil", inProcess, err)
	}
}

func TestBuildOptionsTagsAndUniqueTag(t *testing.T) {
	cfg := testNATSConfig(t)
	cfg.ServerTags = map[string]string{"zone": "a", "rack": "r1"}
	cfg.JetStream.UniqueTag = "zone"

	opts, err := BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	tags := []string(opts.Tags)
	slices.Sort(tags)
	if !slices.Equal([]string{"rack:r1", "zone:a"}, tags) {
		t.Errorf("tags = %v", tags)
	}
	if "zone" != opts.JetStreamUniqueTag {
		t.Errorf("unique tag = %q, want zone", opts.JetStreamUniqueTag)
	}
}

func TestBuildOptionsMonitor(t *testing.T) {
	cfg := testNATSConfig(t)
	opts, err := BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	if "" != opts.HTTPHost || 0 != opts.HTTPPort {
		t.Errorf("monitor enabled without monitor_listen: %s:%d", opts.HTTPHost, opts.HTTPPort)
	}

	cfg.MonitorListen = "127.0.0.1:8222"
	opts, err = BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	if "127.0.0.1" != opts.HTTPHost || 8222 != opts.HTTPPort {
		t.Errorf("monitor = %s:%d, want 127.0.0.1:8222", opts.HTTPHost, opts.HTTPPort)
	}

	cfg.MonitorListen = "no-port"
	if _, err := BuildOptions(cfg, nil); nil == err {
		t.Error("invalid monitor_listen accepted")
	}
}

func TestResolveClientIdentityModes(t *testing.T) {
	secret := "test-secret-0123456789abcdef"
	derived, err := resolveClientIdentity(config.NATSAuthConfig{Mode: config.NATSAuthDerived}, secret)
	if nil != err {
		t.Fatalf("derived: %v", err)
	}
	creds, err := DeriveCredentials(secret)
	if nil != err {
		t.Fatalf("derive: %v", err)
	}
	if creds.UserPublic != derived.public || 1 != len(derived.options) {
		t.Errorf("derived identity = %q with %d options", derived.public, len(derived.options))
	}

	kp, err := nkeys.CreateUser()
	if nil != err {
		t.Fatalf("create user nkey: %v", err)
	}
	seed, err := kp.Seed()
	if nil != err {
		t.Fatalf("read seed: %v", err)
	}
	seedFile := filepath.Join(t.TempDir(), "user.nk")
	if err := os.WriteFile(seedFile, seed, 0o600); nil != err {
		t.Fatalf("write seed: %v", err)
	}
	wantPublic, err := kp.PublicKey()
	if nil != err {
		t.Fatalf("public key: %v", err)
	}
	nkeyIdentity, err := resolveClientIdentity(config.NATSAuthConfig{Mode: config.NATSAuthNKey, NKeySeedFile: seedFile}, secret)
	if nil != err {
		t.Fatalf("nkey: %v", err)
	}
	if wantPublic != nkeyIdentity.public || 1 != len(nkeyIdentity.options) {
		t.Errorf("nkey identity = %q, want %q", nkeyIdentity.public, wantPublic)
	}

	if _, err := resolveClientIdentity(config.NATSAuthConfig{Mode: config.NATSAuthNKey, NKeySeedFile: filepath.Join(t.TempDir(), "missing.nk")}, secret); nil == err {
		t.Error("missing nkey seed file accepted")
	}

	credsIdentity, err := resolveClientIdentity(config.NATSAuthConfig{Mode: config.NATSAuthCreds, CredsFile: "callfs.creds"}, secret)
	if nil != err {
		t.Fatalf("creds: %v", err)
	}
	if "" != credsIdentity.public || 1 != len(credsIdentity.options) {
		t.Errorf("creds identity = %q with %d options", credsIdentity.public, len(credsIdentity.options))
	}

	opts := &server.Options{}
	if err := applyServerAuth(opts, testNATSConfig(t), secret, credsIdentity); nil == err {
		t.Error("embedded server accepted creds identity")
	}

	cfg := testNATSConfig(t)
	cfg.Auth = config.NATSAuthConfig{Mode: config.NATSAuthNKey, NKeySeedFile: seedFile}
	opts = &server.Options{}
	if err := applyServerAuth(opts, cfg, secret, nkeyIdentity); nil != err {
		t.Fatalf("apply nkey auth: %v", err)
	}
	if 1 != len(opts.Nkeys) || wantPublic != opts.Nkeys[0].Nkey {
		t.Errorf("authorized nkeys = %v, want only %s", opts.Nkeys, wantPublic)
	}
}

type fakeRouteClient struct {
	opts server.ClientOpts
}

func (f *fakeRouteClient) GetOpts() *server.ClientOpts                 { return &f.opts }
func (f *fakeRouteClient) GetTLSConnectionState() *tls.ConnectionState { return nil }
func (f *fakeRouteClient) RegisterUser(*server.User)                   {}
func (f *fakeRouteClient) RemoteAddress() net.Addr                     { return nil }
func (f *fakeRouteClient) GetNonce() []byte                            { return nil }
func (f *fakeRouteClient) Kind() int                                   { return server.ROUTER }
func (f *fakeRouteClient) GetID() uint64                               { return 1 }

func TestApplyServerAuthAcceptsPreviousSecret(t *testing.T) {
	current := "current-secret-0123456789abcdef"
	previous := "previous-secret-0123456789abcdef"
	stranger := "stranger-secret-0123456789abcdef"

	cfg := testNATSConfig(t)
	cfg.Routes = []string{"nats-route://127.0.0.1:16222"}
	cfg.Auth.PreviousSecret = previous

	opts, err := BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	identity, err := resolveClientIdentity(cfg.Auth, current)
	if nil != err {
		t.Fatalf("identity: %v", err)
	}
	if err := applyServerAuth(opts, cfg, current, identity); nil != err {
		t.Fatalf("apply auth: %v", err)
	}

	currentCreds, err := DeriveCredentials(current)
	if nil != err {
		t.Fatalf("derive current: %v", err)
	}
	previousCreds, err := DeriveCredentials(previous)
	if nil != err {
		t.Fatalf("derive previous: %v", err)
	}
	strangerCreds, err := DeriveCredentials(stranger)
	if nil != err {
		t.Fatalf("derive stranger: %v", err)
	}

	authorized := make([]string, 0, len(opts.Nkeys))
	for _, user := range opts.Nkeys {
		authorized = append(authorized, user.Nkey)
	}
	if !slices.Contains(authorized, currentCreds.UserPublic) || !slices.Contains(authorized, previousCreds.UserPublic) {
		t.Errorf("authorized nkeys %v miss current or previous user", authorized)
	}
	if slices.Contains(authorized, strangerCreds.UserPublic) {
		t.Error("stranger nkey authorized")
	}

	if 1 != len(opts.Routes) {
		t.Fatalf("routes = %d, want 1", len(opts.Routes))
	}
	routePassword, _ := opts.Routes[0].User.Password()
	if currentCreds.RouteUser != opts.Routes[0].User.Username() || currentCreds.RoutePassword != routePassword {
		t.Error("outgoing route does not use the current route password")
	}

	if nil == opts.CustomRouterAuthentication {
		t.Fatal("route authenticator not set")
	}
	for name, creds := range map[string]Credentials{"current": currentCreds, "previous": previousCreds} {
		client := &fakeRouteClient{opts: server.ClientOpts{Username: creds.RouteUser, Password: creds.RoutePassword}}
		if !opts.CustomRouterAuthentication.Check(client) {
			t.Errorf("%s route password rejected", name)
		}
	}
	rejected := &fakeRouteClient{opts: server.ClientOpts{Username: strangerCreds.RouteUser, Password: strangerCreds.RoutePassword}}
	if opts.CustomRouterAuthentication.Check(rejected) {
		t.Error("stranger route password accepted")
	}
	wrongUser := &fakeRouteClient{opts: server.ClientOpts{Username: "other", Password: currentCreds.RoutePassword}}
	if opts.CustomRouterAuthentication.Check(wrongUser) {
		t.Error("route with wrong user accepted")
	}
}

func TestApplyServerAuthWithoutPreviousSecret(t *testing.T) {
	secret := "current-secret-0123456789abcdef"
	cfg := testNATSConfig(t)
	opts, err := BuildOptions(cfg, nil)
	if nil != err {
		t.Fatalf("build options: %v", err)
	}
	identity, err := resolveClientIdentity(cfg.Auth, secret)
	if nil != err {
		t.Fatalf("identity: %v", err)
	}
	if err := applyServerAuth(opts, cfg, secret, identity); nil != err {
		t.Fatalf("apply auth: %v", err)
	}
	if 1 != len(opts.Nkeys) || identity.public != opts.Nkeys[0].Nkey {
		t.Errorf("authorized nkeys = %v, want only the current user", opts.Nkeys)
	}
}
