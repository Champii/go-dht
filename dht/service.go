package dht

import (
	"context"
	"encoding/hex"
)

type Service struct {
	Dht *Dht
}

func (this *Service) addNode(contact PacketContact) *Node {
	node, err := this.Dht.routing.GetByAddr(contact.Addr)

	if err != nil && compare(contact.Hash, this.Dht.hash) != 0 {
		node = NewNode(this.Dht, contact)

		this.Dht.routing.AddNode(node)
	}

	return node
}

func (this *Service) Ping(ctx context.Context, req *PacketHeader, res *Response) error {
	node := this.addNode(req.Sender)

	this.Dht.logger.Debug(node, "> PING")

	*res = *NewEmptyResponse(this.Dht)

	res.Ok = true

	this.Dht.logger.Debug(node, "< PONG")

	return nil
}

func (this *Service) FetchNodes(ctx context.Context, req *FetchRequest, res *Response) error {
	node := this.addNode(req.Header.Sender)

	this.Dht.logger.Debug(node, "> FETCH NODES", hex.EncodeToString(req.Hash))

	*res = *NewEmptyResponse(this.Dht)

	bucket := this.Dht.routing.FindNode(req.Hash)

	var nodesContact []PacketContact

	for _, contact := range bucket {
		nodesContact = append(nodesContact, contact.Contact)
	}

	res.Contacts = nodesContact

	this.Dht.logger.Debug(node, "< FOUND NODES", len(res.Contacts))

	return nil
}

func (this *Service) Fetch(ctx context.Context, req *FetchRequest, res *Response) error {
	node := this.addNode(req.Header.Sender)

	this.Dht.logger.Debug(node, "> FETCH", hex.EncodeToString(req.Hash))

	*res = *NewEmptyResponse(this.Dht)

	val, ok := this.Dht.store[hex.EncodeToString(req.Hash)]

	if ok {
		res.Data = val

		this.Dht.logger.Debug(node, "< FOUND", hex.EncodeToString(req.Hash), len(res.Data))

		return nil
	}

	if err := this.FetchNodes(ctx, req, res); err != nil {
		return err
	}

	this.Dht.logger.Debug(node, "< FOUND NODES", len(res.Contacts))

	return nil
}

func (this *Service) Store(ctx context.Context, req *StoreRequest, res *Response) error {
	node := this.addNode(req.Header.Sender)

	this.Dht.logger.Debug(node, "> STORE", hex.EncodeToString(req.Hash), len(req.Data))

	*res = *NewEmptyResponse(this.Dht)

	this.Dht.Lock()
	_, ok := this.Dht.store[hex.EncodeToString(req.Hash)]
	this.Dht.Unlock()

	itemSize := len(req.Data)
	storageSize := this.Dht.StorageSize()

	if ok ||
		!this.Dht.onStore(req) ||
		itemSize > this.Dht.options.MaxItemSize ||
		itemSize+storageSize > this.Dht.options.MaxStorageSize {

		res.Ok = false

		this.Dht.logger.Debug(node, "< NOT STORED")

		return nil
	}

	this.Dht.Lock()
	this.Dht.store[hex.EncodeToString(req.Hash)] = req.Data
	this.Dht.Unlock()

	res.Ok = true

	this.Dht.logger.Debug(node, "< STORED", hex.EncodeToString(req.Hash))

	return nil
}
