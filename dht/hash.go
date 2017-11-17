package dht

import (
	"crypto/rand"
	"crypto/sha256"
)

const (
	HASH_SIZE   = 128
	BUCKET_SIZE = HASH_SIZE / 8
)

func NewHash(val []byte) []byte {
	h := sha256.New()

	h.Write(val)

	return h.Sum(nil)[:BUCKET_SIZE]
}

func NewRandomHash() []byte {
	res := make([]byte, BUCKET_SIZE)

	rand.Read(res)

	return res[:BUCKET_SIZE]
}
