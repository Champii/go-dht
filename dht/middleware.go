package dht

type IMiddleware interface {
	Init(*Dht) error
	BeforeSendStore(*StoreRequest) *StoreRequest
	OnStore(*StoreRequest) bool
	// OnCustomCmd(Packet) interface{}
	// OnBroadcast(Packet) interface{}
}

func (this *Dht) Use(m IMiddleware) {
	if err := m.Init(this); err != nil {
		panic(err)
	}

	this.middlewares = append(this.middlewares, m)
}
