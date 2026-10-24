package natsrv

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"golang.org/x/crypto/hkdf"
)

type Credentials struct {
	User          nkeys.KeyPair
	UserPublic    string
	RouteUser     string
	RoutePassword string
}

func DeriveCredentials(secret string) (Credentials, error) {
	if "" == secret {
		return Credentials{}, fmt.Errorf("nats: internal secret is empty")
	}

	userSeed, err := deriveKeyMaterial(secret, "callfs/nats/user/v1")
	if nil != err {
		return Credentials{}, fmt.Errorf("nats: derive user seed: %w", err)
	}

	kp, err := nkeys.FromRawSeed(nkeys.PrefixByteUser, userSeed)
	if nil != err {
		return Credentials{}, fmt.Errorf("nats: build user key pair: %w", err)
	}

	pub, err := kp.PublicKey()
	if nil != err {
		return Credentials{}, fmt.Errorf("nats: read user public key: %w", err)
	}

	routeMaterial, err := deriveKeyMaterial(secret, "callfs/nats/route/v1")
	if nil != err {
		return Credentials{}, fmt.Errorf("nats: derive route password: %w", err)
	}

	return Credentials{
		User:          kp,
		UserPublic:    pub,
		RouteUser:     "callfs-route",
		RoutePassword: fmt.Sprintf("%x", routeMaterial),
	}, nil
}

func deriveKeyMaterial(secret string, info string) ([]byte, error) {
	reader := hkdf.New(sha256.New, []byte(secret), nil, []byte(info))
	material := make([]byte, 32)
	if _, err := io.ReadFull(reader, material); nil != err {
		return nil, err
	}
	return material, nil
}

type clientIdentity struct {
	public  string
	options []nats.Option
}

func resolveClientIdentity(auth config.NATSAuthConfig, secret string) (clientIdentity, error) {
	switch auth.Mode {
	case config.NATSAuthNKey:
		return nkeyIdentity(auth.NKeySeedFile)
	case config.NATSAuthCreds:
		if "" == auth.CredsFile {
			return clientIdentity{}, errors.New("nats: nats.auth.creds_file is empty")
		}
		return clientIdentity{options: []nats.Option{nats.UserCredentials(auth.CredsFile)}}, nil
	case config.NATSAuthDerived, "":
		creds, err := DeriveCredentials(secret)
		if nil != err {
			return clientIdentity{}, err
		}
		return clientIdentity{
			public:  creds.UserPublic,
			options: []nats.Option{nats.Nkey(creds.UserPublic, creds.User.Sign)},
		}, nil
	default:
		return clientIdentity{}, fmt.Errorf("nats: unknown auth mode %q", auth.Mode)
	}
}

func nkeyIdentity(seedFile string) (clientIdentity, error) {
	raw, err := os.ReadFile(seedFile)
	if nil != err {
		return clientIdentity{}, fmt.Errorf("nats: read nats.auth.nkey_seed_file: %w", err)
	}
	kp, err := nkeys.ParseDecoratedNKey(raw)
	if nil != err {
		return clientIdentity{}, fmt.Errorf("nats: parse nats.auth.nkey_seed_file: %w", err)
	}
	pub, err := kp.PublicKey()
	if nil != err {
		return clientIdentity{}, fmt.Errorf("nats: read nkey public key: %w", err)
	}
	if !nkeys.IsValidPublicUserKey(pub) {
		return clientIdentity{}, errors.New("nats: nats.auth.nkey_seed_file must hold a user nkey seed")
	}
	return clientIdentity{
		public:  pub,
		options: []nats.Option{nats.Nkey(pub, kp.Sign)},
	}, nil
}

func applyServerAuth(opts *server.Options, cfg config.NATSConfig, secret string, identity clientIdentity) error {
	if "" == identity.public {
		return errors.New("nats: the embedded server needs nats.auth.mode derived or nkey")
	}
	current, err := DeriveCredentials(secret)
	if nil != err {
		return err
	}

	accepted := []string{identity.public}
	passwords := []string{current.RoutePassword}
	if "" != cfg.Auth.PreviousSecret {
		previous, err := DeriveCredentials(cfg.Auth.PreviousSecret)
		if nil != err {
			return fmt.Errorf("nats: derive previous credentials: %w", err)
		}
		passwords = append(passwords, previous.RoutePassword)
		if config.NATSAuthDerived == cfg.Auth.Mode && previous.UserPublic != identity.public {
			accepted = append(accepted, previous.UserPublic)
		}
	}

	opts.Nkeys = make([]*server.NkeyUser, 0, len(accepted))
	for _, public := range accepted {
		opts.Nkeys = append(opts.Nkeys, &server.NkeyUser{Nkey: public})
	}
	opts.Cluster.Username = current.RouteUser
	opts.Cluster.Password = current.RoutePassword
	opts.CustomRouterAuthentication = routeAuthenticator{user: current.RouteUser, passwords: passwords}
	for _, route := range opts.Routes {
		route.User = url.UserPassword(current.RouteUser, current.RoutePassword)
	}
	return nil
}

type routeAuthenticator struct {
	user      string
	passwords []string
}

func (a routeAuthenticator) Check(c server.ClientAuthentication) bool {
	clientOpts := c.GetOpts()
	if nil == clientOpts {
		return false
	}
	if 1 != subtle.ConstantTimeCompare([]byte(a.user), []byte(clientOpts.Username)) {
		return false
	}
	matched := 0
	for _, password := range a.passwords {
		matched |= subtle.ConstantTimeCompare([]byte(password), []byte(clientOpts.Password))
	}
	return 1 == matched
}
