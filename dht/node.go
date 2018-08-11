package dht

import (
	"context"
	"encoding/hex"

	"github.com/smallnest/rpcx/client"
)

type Node struct {
	Dht     *Dht
	Contact PacketContact
	Client  *client.Client
}

func NewNode(d *Dht, contact PacketContact) *Node {
	return &Node{
		Dht:     d,
		Contact: contact,
	}
}

func (this *Node) Connect() error {
	if this.Client != nil {
		return nil
	}

	option := client.DefaultOption
	option.Block = bc

	xclient := client.NewClient(option)

	if err := xclient.Connect("kcp", this.Contact.Addr); err != nil {
		this.Dht.logger.Error(err)

		return err
	}

	this.Client = xclient
	return nil
}

func (this *Node) Ping() *Response {
	this.Connect()

	this.Dht.logger.Debug(this, "< PING")

	req := NewHeader(this.Dht)
	res := &Response{}

	err := this.Client.Call(context.Background(), "DhtService", "Ping", &req, res)

	if err != nil {
		res.Err = err

		this.Dht.logger.Debug(this, "! ", res.Err)

		this.Dht.routing.RemoveNode(this)

		return res
	}

	this.afterResponse(&res.Header)

	this.Dht.logger.Debug(this, "> PONG")

	return res
}

func (this *Node) FetchNodes(hash []byte) *Response {
	this.Connect()

	this.Dht.logger.Debug(this, "< FETCH NODES", hex.EncodeToString(hash))

	req := NewFetchRequest(this.Dht, hash)
	res := &Response{}

	err := this.Client.Call(context.Background(), "DhtService", "FetchNodes", req, res)

	if err != nil {
		res.Err = err

		this.Dht.logger.Debug(this, "! ", res.Err)

		this.Dht.routing.RemoveNode(this)

		return res
	}

	this.Dht.logger.Debug(this, "> FOUND NODES", len(res.Contacts))

	this.afterResponse(&res.Header)

	return res
}

func (this *Node) Fetch(hash []byte) *Response {
	this.Connect()

	this.Dht.logger.Debug(this, "< FETCH", hex.EncodeToString(hash))

	req := NewFetchRequest(this.Dht, hash)
	res := &Response{}

	err := this.Client.Call(context.Background(), "DhtService", "Fetch", req, res)

	if err != nil {
		res.Err = err

		this.Dht.logger.Debug(this, "! ", res.Err)

		this.Dht.routing.RemoveNode(this)

		return res
	}

	if len(res.Contacts) > 0 {
		this.Dht.logger.Debug(this, "> FOUND NODES", len(res.Contacts))
	} else {
		this.Dht.logger.Debug(this, "> FOUND", hex.EncodeToString(hash), len(res.Data))
	}

	this.afterResponse(&res.Header)

	return res
}

func (this *Node) Store(hash []byte, data []byte) *Response {
	this.Connect()

	req := NewStoreRequest(this.Dht, hash, data)
	res := &Response{}

	req = this.Dht.beforeSendStore(req)

	this.Dht.logger.Debug(this, "< STORE", hex.EncodeToString(hash), len(req.Data))

	err := this.Client.Call(context.Background(), "DhtService", "Store", req, res)

	if err != nil {
		res.Err = err

		this.Dht.logger.Debug(this, "! ", res.Err)

		this.Dht.routing.RemoveNode(this)

		return res
	}

	if res.Ok {
		this.Dht.logger.Debug(this, "> STORED", hex.EncodeToString(hash), len(req.Data))
	} else {
		this.Dht.logger.Debug(this, "> NOT STORED", hex.EncodeToString(hash))
	}

	this.afterResponse(&res.Header)

	return res
}

func (this *Node) CustomCmd(data interface{}) *CustomResponse {
	this.Connect()

	req := NewCustomRequest(this.Dht, data)
	res := &CustomResponse{}

	this.Dht.logger.Debug(this, "< CUSTOM CMD")

	err := this.Client.Call(context.Background(), "DhtService", "CustomCmd", req, res)

	if err != nil {
		res.Err = err

		this.Dht.logger.Debug(this, "! ", res.Err)

		this.Dht.routing.RemoveNode(this)

		return res
	}

	if res.Ok {
		this.Dht.logger.Debug(this, "> CUSTOM CMD OK")
	} else {
		this.Dht.logger.Debug(this, "> CUSTOM CMD NOT OK")
	}

	this.afterResponse(&res.Header)

	return res
}

func (this *Node) afterResponse(res *PacketHeader) {
	this.Contact = res.Sender

	if compare(this.Contact.Hash, this.Dht.hash) == 0 {
		return
	}

	this.Dht.routing.AddNode(this)
}

func (this *Node) Redacted() interface{} {
	if len(this.Contact.Hash) == 0 {
		return this.Contact.Addr
	}

	return hex.EncodeToString(this.Contact.Hash)[:16]
}
