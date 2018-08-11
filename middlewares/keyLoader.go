package middleware

import (
	"babble/crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/champii/go-dht/dht"
	// "strings"
)

type Key struct {
	name string
	key  *ecdsa.PrivateKey
	pub  []byte
}

func (this *Key) Name() string {
	return this.name
}

func (this *Key) Pub() []byte {
	return this.pub
}

func (this *Key) Priv() *ecdsa.PrivateKey {
	return this.key
}

type KeyOpt struct {
	Folder string
}

func (this *AuthMiddleware) GetKeys(options KeyOpt) error {
	keys, err := ioutil.ReadDir(options.Folder + "/keys")

	if err != nil {
		if err := os.MkdirAll(options.Folder+"/keys", 0755); err != nil {
			return err
		}
	}

	for _, key := range keys {
		blob, err := ioutil.ReadFile(options.Folder + "/keys/" + key.Name())

		if err != nil {
			this.Logger.Warning("Key", key.Name(), "is not readable", err)

			return err
		}

		block, _ := pem.Decode(blob)
		x509Encoded := block.Bytes
		privateKey, err := x509.ParseECPrivateKey(x509Encoded)

		if err != nil {
			this.Logger.Warning("Key", key.Name(), "is corrupted !", err)

			return err
		}

		x509EncodedPub, err := x509.MarshalPKIXPublicKey(privateKey.Public())

		if err != nil {
			return err
		}

		pemEncodedPub := pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: x509EncodedPub,
		})

		this.Logger.Info("Loaded key", key.Name(), SanitizePubKey(pemEncodedPub))

		this.Keys[key.Name()] = &Key{
			name: key.Name(),
			key:  privateKey,
			pub:  crypto.FromECDSAPub(&privateKey.PublicKey),
		}

	}

	return nil
}

func (this *AuthMiddleware) CreateKey(name string, options KeyOpt) (*Key, error) {
	stat, err := os.Stat(options.Folder + "/keys")

	if err != nil {
		if err = os.Mkdir(options.Folder+"/keys", 0755); err != nil {
			return nil, err
		}
	} else {
		if !stat.IsDir() {
			return nil, errors.New(options.Folder + "/keys" + " is not a folder")
		}
	}

	keyPath := options.Folder + "/keys/" + name + ".key"
	_, err = os.Stat(keyPath)

	if err == nil {
		return nil, errors.New("Existing key " + name + ".key")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	if err != nil {
		return nil, err
	}

	priv, err := x509.MarshalECPrivateKey(key)

	if err != nil {
		return nil, err
	}

	x509EncodedPub, err := x509.MarshalPKIXPublicKey(key.Public())

	if err != nil {
		return nil, err
	}

	pemEncodedPub := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: x509EncodedPub,
	})

	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv})

	err = ioutil.WriteFile(keyPath, pemEncoded, 0600)

	if err != nil {
		return nil, err
	}

	this.Logger.Info("Created key", name+".key", SanitizePubKey(pemEncodedPub))

	fmt.Println("OUECH", pemEncoded)
	return &Key{
		name: name + ".key",
		key:  key,
		pub:  crypto.FromECDSAPub(&key.PublicKey),
	}, nil
}

func SanitizePubKey(pub []byte) string {
	return dht.NewHash(pub).String()
}
