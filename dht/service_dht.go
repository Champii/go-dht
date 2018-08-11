package dht

import (
	"context"
	"encoding/hex"

	metrics "github.com/rcrowley/go-metrics"
)

var storeCounter = metrics.NewGauge()
var storeAmount = metrics.NewGauge()

type IService interface {
	GetService() IService
	Name() string
}

type DhtService struct {
	Dht *Dht
}

func (this *DhtService) GetService() *DhtService {
	return this
}

func (this *DhtService) Name() string {
	return "DhtService"
}

func (this *DhtService) addNode(contact PacketContact) *Node {
	node, err := this.Dht.routing.GetByAddr(contact.Addr)

	if err != nil && compare(contact.Hash, this.Dht.hash) != 0 {
		node = NewNode(this.Dht, contact)

		this.Dht.routing.AddNode(node)
	}

	return node
}

func (this *DhtService) Ping(ctx context.Context, req *PacketHeader, res *Response) error {
	node := this.addNode(req.Sender)

	this.Dht.logger.Debug(node, "> PING")

	*res = *NewEmptyResponse(this.Dht)

	res.Ok = true

	this.Dht.logger.Debug(node, "< PONG")

	return nil
}

func (this *DhtService) FetchNodes(ctx context.Context, req *FetchRequest, res *Response) error {
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

func (this *DhtService) Fetch(ctx context.Context, req *FetchRequest, res *Response) error {
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

func (this *DhtService) Store(ctx context.Context, req *StoreRequest, res *Response) error {
	node := this.addNode(req.Header.Sender)

	this.Dht.logger.Debug(node, "> STORE", hex.EncodeToString(req.Hash), len(req.Data))

	*res = *NewEmptyResponse(this.Dht)

	itemSize := len(req.Data)
	storageSize := this.Dht.StorageSize()

	if !this.Dht.onStore(req) ||
		itemSize > this.Dht.options.MaxItemSize ||
		itemSize+storageSize > this.Dht.options.MaxStorageSize {

		res.Ok = false

		this.Dht.logger.Debug(node, "< NOT STORED")

		return nil
	}

	this.Dht.Lock()
	this.Dht.store[hex.EncodeToString(req.Hash)] = req.Data

	metrics.GetOrRegister("storedElements", storeCounter)
	metrics.GetOrRegister("storedAmount", storeAmount)
	storeCounter.Update(int64(len(this.Dht.store)))
	this.Dht.Unlock()

	storeAmount.Update(int64(this.Dht.StorageSize() / 1024))

	res.Ok = true

	this.Dht.logger.Debug(node, "< STORED", hex.EncodeToString(req.Hash))

	return nil
}

func (this *DhtService) CustomCmd(ctx context.Context, req *CustomRequest, res *CustomResponse) error {
	*res = *NewEmptyCustomResponse(this.Dht)

	resData, err := this.Dht.onCustomCmd(req.Data)

	if err != nil {
		return err
	}

	res.Data = resData

	return nil
}
