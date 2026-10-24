package natsrv

import (
	"crypto/sha256"
	"fmt"
	"io"

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
