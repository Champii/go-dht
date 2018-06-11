package middleware

import (
	"go-dht/dht"
)

type AuthMiddleware struct {
}

func (this *AuthMiddleware) Init() error {

	return nil
}

func (this *AuthMiddleware) BeforeSend(p dht.Packet) dht.Packet {

	return p
}

func (this *AuthMiddleware) OnStore(dht.Packet) bool {
	return true
}

func (this *AuthMiddleware) OnCustomCmd(dht.Packet) interface{} {
	return nil
}

func (this *AuthMiddleware) OnBroadcast(dht.Packet) interface{} {
	return nil
}
