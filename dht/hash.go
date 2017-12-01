package dht

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const (
	HASH_SIZE   = 128
	BUCKET_SIZE = HASH_SIZE / 8
)

type Hash []byte

func (this Hash) Redacted() interface{} {
	if len(this) == 0 {
		return this
	}

	return hex.EncodeToString(this)[:16]
}

func NewHash(val []byte) Hash {
	h := sha256.New()

	h.Write(val)

	return h.Sum(nil)[:BUCKET_SIZE]
}

func NewRandomHash() Hash {
	res := make([]byte, BUCKET_SIZE)

	rand.Read(res)

	return res[:BUCKET_SIZE]
}
