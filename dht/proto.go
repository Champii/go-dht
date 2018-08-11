package dht

import "time"

type Request interface {
	GetRequest()
}

type PacketContact struct {
	Hash Hash
	Addr string
}

type PacketHeader struct {
	DateSent time.Time
	Sender   PacketContact
	MsgHash  string // maybe []byte here
}

type FetchRequest struct {
	Header PacketHeader
	Hash   []byte
}

func (this FetchRequest) GetRequest() {

}

type StoreRequest struct {
	Header PacketHeader
	Hash   []byte
	Data   []byte
}

func (this StoreRequest) GetRequest() {

}

type CustomRequest struct {
	Header PacketHeader
	Data   interface{}
}

func (this CustomRequest) GetRequest() {

}

type Response struct {
	Header   PacketHeader
	Contacts []PacketContact
	Data     []byte
	Ok       bool
	Err      error
}

type CustomResponse struct {
	Header PacketHeader
	Data   interface{}
	Ok     bool
	Err    error
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

func NewCustomRequest(d *Dht, data interface{}) *CustomRequest {
	return &CustomRequest{
		Header: NewHeader(d),
		Data:   data,
	}
}

func NewEmptyResponse(d *Dht) *Response {
	return &Response{
		Header: NewHeader(d),
	}
}

func NewEmptyCustomResponse(d *Dht) *CustomResponse {
	return &CustomResponse{
		Header: NewHeader(d),
	}
}
