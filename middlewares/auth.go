package middleware

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"go-dht/dht"
	"math/big"
	"time"

	logging "github.com/op/go-logging"
	"github.com/vmihailenco/msgpack"
)

type Stamp struct {
	R         []byte
	S         []byte
	Pub       []byte
	Hash      []byte
	Timestamp int64
}

type AuthStoreRequest struct {
	Stamp Stamp
	Data  []byte
}

type AuthMiddleware struct {
	Dht    *dht.Dht
	Logger *logging.Logger
	Keys   map[string]*Key
}

func (this *AuthMiddleware) Init(d *dht.Dht) error {
	this.Keys = make(map[string]*Key)
	this.Dht = d
	this.Logger = d.Logger()

	this.CreateKey("main", KeyOpt{"."})

	return this.GetKeys(KeyOpt{"."})
}

func (this *AuthMiddleware) BeforeSendStore(req *dht.StoreRequest) *dht.StoreRequest {
	stamp := Stamp{
		Pub:       this.Keys["main.key"].pub,
		Timestamp: time.Now().Unix(),
		Hash:      []byte{},
		R:         []byte{},
		S:         []byte{},
	}

	hash, err := msgpack.Marshal(req)

	if err != nil {
		return nil
	}

	newHash := dht.NewHash(hash)
	stamp.Hash = newHash

	r, s, err := ecdsa.Sign(rand.Reader, this.Keys["main.key"].key, newHash)

	if err != nil {
		this.Logger.Warning("Cannot create transaction: Signature error", err)

		return nil
	}

	stamp.R = r.Bytes()
	stamp.S = s.Bytes()

	newReq := AuthStoreRequest{
		Stamp: stamp,
		Data:  req.Data,
	}

	msgSerial, err := msgpack.Marshal(newReq)

	if err != nil {
		return nil
	}

	req.Data = msgSerial

	return req
}

func (this *AuthMiddleware) OnStore(req *dht.StoreRequest) bool {
	authReq := &AuthStoreRequest{}
	err := msgpack.Unmarshal(req.Data, &authReq)

	if err != nil {
		return false
	}

	req.Data = authReq.Data

	hash, err := msgpack.Marshal(req)

	if err != nil {
		return false
	}

	newHash := dht.NewHash(hash)

	if dht.Compare(newHash, authReq.Stamp.Hash) != 0 {
		this.Logger.Error("verify: Hash dont match", newHash, authReq.Stamp.Hash)

		return false
	}

	blockPub, _ := pem.Decode(authReq.Stamp.Pub)

	if blockPub == nil {
		this.Logger.Error("verify: Cannot decode pub signature from tx", string(authReq.Stamp.Pub))

		return false
	}

	// verify timestamp

	x509EncodedPub := blockPub.Bytes
	genericPublicKey, _ := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*ecdsa.PublicKey)

	var r_ big.Int
	// var r2 *big.Int
	r_.SetBytes(authReq.Stamp.R)

	var s_ big.Int
	s_.SetBytes(authReq.Stamp.S)

	if !ecdsa.Verify(publicKey, newHash, &r_, &s_) {
		this.Logger.Error("verify: Signatures does not match")

		return false
	}

	req.Data = authReq.Data

	return true
}

// func (this *AuthMiddleware) OnCustomCmd(dht.Packet) interface{} {
// 	return nil
// }

// func (this *AuthMiddleware) OnBroadcast(dht.Packet) interface{} {
// 	return nil
// }
