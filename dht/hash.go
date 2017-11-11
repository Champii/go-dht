package dht

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const (
	HASH_SIZE   = 256
	BUCKET_SIZE = HASH_SIZE / 8
)

func NewHash(val []byte) string {
	h := sha256.New()

	h.Write(val)

	return hex.EncodeToString(h.Sum(nil))
}

func NewRandomHash() string {
	res := make([]byte, BUCKET_SIZE)

	rand.Read(res)

	return hex.EncodeToString(res)
}
