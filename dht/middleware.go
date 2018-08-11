package dht

type IMiddleware interface {
	Init(*Dht) error
	OnAddNode(*Node) bool
	BeforeSendStore(*StoreRequest) *StoreRequest
	OnStore(*StoreRequest) bool
	OnCustomCmd(interface{}) (interface{}, error)
	// OnBroadcast(Packet) interface{}
}

func (this *Dht) Use(m IMiddleware) {
	if err := m.Init(this); err != nil {
		panic(err)
	}

	this.middlewares = append(this.middlewares, m)
}
