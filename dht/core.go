package dht

import (
	"encoding/hex"
	"errors"
)

func (this *Dht) bootstrap() error {
	this.logger.Debug("Connecting to bootstrap node", this.options.BootstrapAddr)

	node := NewNode(this, PacketContact{
		Addr: this.options.BootstrapAddr,
	})

	// this.routing.AddNode(node)

	res := node.Ping()

	if res.Err != nil || !res.Ok {
		return res.Err
	}

	this.fetchNodes(this.hash)

	// for i, bucket := range this.routing.buckets {
	// 	if len(bucket) != 0 {
	// 		continue
	// 	}

	// 	h := NewRandomHash()
	// 	h = this.routing.nCopy(h, this.hash, i)

	// 	this.fetchNodes(h)
	// }

	this.logger.Info("Ready...")

	if this.options.Interactif {
		go this.Cli()
	}

	return nil
}

func (this *Dht) republish() {
	for k, v := range this.store {
		h, _ := hex.DecodeString(k)
		this.StoreAt(h, v)
	}

	this.logger.Debug("Republished", len(this.store))
}

func (this *Dht) Store(value []byte) (Hash, error) {
	return this.StoreAt(NewHash(value), value)
}

func (this *Dht) StoreAt(hash Hash, value []byte) (Hash, error) {
	if len(value) > this.options.MaxItemSize {
		return []byte{}, errors.New("Store: Exceeds max limit")
	}

	bucket, err := this.fetchNodes(hash)

	if err != nil {
		return []byte{}, err
	}

	if len(bucket) == 0 {
		return []byte{}, errors.New("No nodes found")
	}

	fn := func(node *Node) *Response {
		return node.Store(hash, value)
	}

	query := NewQuery(hash, fn, this)

	res := query.Run()

	if res.Err != nil {
		return []byte{}, res.Err
	}

	if res.Ok != true {
		return []byte{}, errors.New("Cannot store")
	}

	return hash, nil
}

func (this *Dht) Fetch(hash Hash) ([]byte, error) {
	fn := func(node *Node) *Response {
		return node.Fetch(hash)
	}

	query := NewQuery(hash, fn, this)

	res := query.Run()

	if res.Err != nil {
		return nil, res.Err
	}

	if len(res.Data) > 0 {
		return res.Data, nil
	}

	return nil, errors.New("Not found")
}

func (this *Dht) fetchNodes(hash Hash) ([]PacketContact, error) {
	fn := func(node *Node) *Response {
		return node.FetchNodes(hash)
	}

	res := NewQuery(hash, fn, this).Run()

	if res.Err != nil {
		return []PacketContact{}, res.Err
	}

	return res.Contacts, nil
}
