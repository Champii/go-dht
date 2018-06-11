package dht

import "time"

type PacketContact struct {
	Hash []byte
	Addr string
}

type PacketHeader struct {
	DateSent time.Time
	Sender   PacketContact
}

type FetchRequest struct {
	Header PacketHeader
	Hash   []byte
}

type StoreRequest struct {
	Header PacketHeader
	Hash   []byte
	Data   []byte
}

type Response struct {
	Header   PacketHeader
	Contacts []PacketContact
	Data     []byte
	Ok       bool
	Err      error
}

func NewHeader(d *Dht) PacketHeader {
	return PacketHeader{
		DateSent: time.Now(),
		Sender: PacketContact{
			Hash: d.hash,
			Addr: d.options.ListenAddr,
		},
	}
}

func NewFetchRequest(d *Dht, hash []byte) *FetchRequest {
	return &FetchRequest{
		Header: NewHeader(d),
		Hash:   hash,
	}
}

func NewStoreRequest(d *Dht, hash []byte, data []byte) *StoreRequest {
	return &StoreRequest{
		Header: NewHeader(d),
		Hash:   hash,
		Data:   data,
	}
}

func NewEmptyResponse(d *Dht) *Response {
	return &Response{
		Header: NewHeader(d),
	}
}
