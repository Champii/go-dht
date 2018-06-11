package dht

type IMiddleware interface {
	Init() error
	// BeforeSend(Packet) Packet
	OnStore(StoreRequest) bool
	// OnCustomCmd(Packet) interface{}
	// OnBroadcast(Packet) interface{}
}

func (this *Dht) Use(m IMiddleware) {
	this.middlewares = append(this.middlewares, m)
}
