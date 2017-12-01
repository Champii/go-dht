package dht

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"net"
	"time"

	"github.com/vmihailenco/msgpack"
)

const (
	COMMAND_NOOP = iota
	COMMAND_PING
	COMMAND_PONG
	COMMAND_STORE
	COMMAND_STORED
	COMMAND_FETCH
	COMMAND_FETCH_NODES
	COMMAND_FOUND
	COMMAND_FOUND_NODES
	COMMAND_BROADCAST
	COMMAND_CUSTOM
	COMMAND_CUSTOM_ANSWER
	COMMAND_REPEAT_PLEASE
)

type Callback func(val Packet, err error)

type CallbackChan struct {
	timer *time.Timer
	c     chan interface{}
}

type Node struct {
	contact  PacketContact
	lastSeen int64
	addr     net.Addr
	dht      *Dht
}

type PacketContact struct {
	Hash Hash
	Addr string
}

type PacketHeader struct {
	DateSent    int64
	Command     int
	Sender      PacketContact
	ResponseTo  Hash
	MessageHash Hash
}

type Packet struct {
	Header PacketHeader
	Data   []byte
}

func (this *Packet) GetData(ptr interface{}) error {
	var blob bytes.Buffer
	blob.Write(this.Data)

	dec := gob.NewDecoder(&blob)

	if err := dec.Decode(ptr); err != nil {
		return err
	}

	return nil
}

func (this *Packet) SetData(data interface{}) error {
	var blob bytes.Buffer

	enc := gob.NewEncoder(&blob)

	if err := enc.Encode(data); err != nil {
		return err
	}

	this.Data = blob.Bytes()

	return nil
}

type StoreInst struct {
	Hash Hash
	Data []byte
}

type CustomCmd struct {
	Command int
	Data    []byte
}

func NewPacketUngob(dht *Dht, command int, responseTo Hash, data []byte) Packet {
	addr, err := net.ResolveUDPAddr("udp", dht.options.ListenAddr)

	packet := Packet{
		Header: PacketHeader{
			DateSent:    time.Now().UnixNano(),
			Command:     command,
			ResponseTo:  responseTo,
			MessageHash: []byte{},
			Sender: PacketContact{
				Addr: addr.String(),
				Hash: dht.hash,
			},
		},
		Data: data,
	}

	tmp, err := msgpack.Marshal(&packet)

	if err != nil {
		dht.logger.Warning(err)
	}

	packet.Header.MessageHash = NewHash(tmp)

	return packet
}

func NewPacket(dht *Dht, command int, responseTo Hash, data interface{}) Packet {
	addr, err := net.ResolveUDPAddr("udp", dht.options.ListenAddr)

	packet := Packet{
		Header: PacketHeader{
			DateSent:    time.Now().UnixNano(),
			Command:     command,
			ResponseTo:  responseTo,
			MessageHash: []byte{},
			Sender: PacketContact{
				Addr: addr.String(),
				Hash: dht.hash,
			},
		},
	}

	packet.SetData(data)

	tmp, err := msgpack.Marshal(&packet)

	if err != nil {
		dht.logger.Warning(err)
	}

	packet.Header.MessageHash = NewHash(tmp)

	return packet
}

func (this *Node) newPacket(command int, responseTo []byte, data interface{}) Packet {
	return NewPacket(this.dht, command, responseTo, data)
}

func (this *Node) newPacketUngob(command int, responseTo []byte, data []byte) Packet {
	return NewPacketUngob(this.dht, command, responseTo, data)
}

func NewNodeContact(dht *Dht, addr net.Addr, contact PacketContact) *Node {
	return &Node{
		dht:      dht,
		addr:     addr,
		lastSeen: time.Now().Unix(),
		contact:  contact,
	}
}

func NewNode(dht *Dht, addr net.Addr, hash []byte) *Node {
	return NewNodeContact(dht, addr, PacketContact{
		Addr: addr.String(),
		Hash: hash,
	})
}

func (this *Node) Redacted() interface{} {
	if len(this.contact.Hash) == 0 {
		return this.contact.Addr
	}

	return hex.EncodeToString(this.contact.Hash)[:16]
}

func (this *Node) HandleInPacket(packet Packet) {
	if len(packet.Header.ResponseTo) > 0 {
		this.dht.Lock()
		cb, ok := this.dht.commandQueue[hex.EncodeToString(packet.Header.ResponseTo)]

		if !ok {
			this.dht.logger.Info(this, "x Unknown response: ", hex.EncodeToString(packet.Header.ResponseTo), packet)
			this.dht.Unlock()
			return
		}

		cb.timer.Stop()
		this.dht.Unlock()

		switch packet.Header.Command {
		case COMMAND_NOOP:
			this.dht.logger.Debug(this, "> NOOP")
			cb.c <- packet
		case COMMAND_PONG:
			this.OnPong(packet, cb)
		case COMMAND_FOUND:
			this.OnFound(packet, cb)
		case COMMAND_FOUND_NODES:
			this.OnFoundNodes(packet, cb)
		case COMMAND_STORED:
			this.OnStored(packet, cb)
		case COMMAND_CUSTOM_ANSWER:
			this.OnCustomAnswer(packet, cb)
		default:
			this.dht.logger.Error(this, "x answer: UNKNOWN COMMAND", packet.Header.Command)
			return
		}

		this.dht.Lock()
		close(cb.c)
		delete(this.dht.commandQueue, hex.EncodeToString(packet.Header.ResponseTo))
		this.dht.Unlock()
	} else {
		switch packet.Header.Command {
		case COMMAND_NOOP:
		case COMMAND_PING:
			this.OnPing(packet)
		case COMMAND_FETCH:
			this.OnFetch(packet)
		case COMMAND_FETCH_NODES:
			this.OnFetchNodes(packet)
		case COMMAND_BROADCAST:
			this.OnBroadcast(packet)
		case COMMAND_STORE:
			this.OnStore(packet)
		case COMMAND_CUSTOM:
			this.OnCustom(packet)
		case COMMAND_REPEAT_PLEASE:
			this.OnRepeatPlease(packet)
		default:
			this.dht.logger.Error(this, "x query: UNKNOWN COMMAND", packet.Header.Command)
			return
		}
	}

}

func (this *Node) Ping() chan interface{} {
	this.dht.logger.Debug(this, "< PING")

	return this.send(this.newPacket(COMMAND_PING, []byte{}, []byte{}))
}

func (this *Node) OnPing(packet Packet) {
	this.dht.logger.Debug(this, "> PING")

	this.Pong(packet.Header.MessageHash)
}

func (this *Node) Pong(responseTo []byte) chan interface{} {
	this.dht.logger.Debug(this, "< PONG")

	data := this.newPacket(COMMAND_PONG, responseTo, []byte{})

	return this.send(data)
}

func (this *Node) OnPong(packet Packet, cb CallbackChan) {
	this.dht.logger.Debug(this, "> PONG")

	cb.c <- nil
}

func (this *Node) Fetch(hash []byte) chan interface{} {
	this.dht.logger.Debug(this, "< FETCH", hash)

	data := this.newPacket(COMMAND_FETCH, []byte{}, hash)

	return this.send(data)
}

func (this *Node) OnFetch(packet Packet) {
	this.dht.logger.Debug(this, "> FETCH")

	var hash Hash
	packet.GetData(&hash)

	val, ok := this.dht.store[hex.EncodeToString(hash)]

	if ok {
		this.Found(packet, val)
		return
	}

	this.OnFetchNodes(packet)
}

func (this *Node) FetchNodes(hash []byte) chan interface{} {
	this.dht.logger.Debug(this, "< FETCH NODES")

	data := this.newPacket(COMMAND_FETCH_NODES, []byte{}, hash)

	return this.send(data)
}

func (this *Node) OnFetchNodes(packet Packet) {
	this.dht.logger.Debug(this, "> FETCH NODES")

	bucket := this.dht.routing.FindNode(packet.Data)

	var nodesContact []PacketContact

	for _, contact := range bucket {
		nodesContact = append(nodesContact, contact)
	}

	this.FoundNodes(packet, nodesContact)
}

func (this *Node) FoundNodes(packet Packet, nodesContact []PacketContact) {
	this.dht.logger.Debug(this, "< FOUND NODES")

	data := this.newPacket(COMMAND_FOUND_NODES, packet.Header.MessageHash, nodesContact)

	this.send(data)
}

func (this *Node) OnFoundNodes(packet Packet, done CallbackChan) {
	this.dht.logger.Debug(this, "> FOUND NODES")

	done.c <- packet
}

func (this *Node) Found(packet Packet, value []byte) {
	this.dht.logger.Debug(this, "< FOUND")

	data := this.newPacketUngob(COMMAND_FOUND, packet.Header.MessageHash, value)

	this.send(data)
}

func (this *Node) OnFound(packet Packet, done CallbackChan) {
	this.dht.logger.Debug(this, "> FOUND")

	done.c <- packet
}

func (this *Node) Store(hash Hash, value []byte) chan interface{} {
	this.dht.logger.Debug(this, "< STORE")

	data := this.newPacket(COMMAND_STORE, []byte{}, StoreInst{Hash: hash, Data: value})

	return this.send(data)
}

func (this *Node) OnStore(packet Packet) {
	this.dht.logger.Debug(this, "> STORE")

	var stInst StoreInst
	packet.GetData(&stInst)

	this.dht.Lock()
	_, ok := this.dht.store[hex.EncodeToString(stInst.Hash)]
	this.dht.Unlock()

	itemSize := len(stInst.Data)
	storageSize := this.dht.StorageSize()

	if ok ||
		!this.dht.onStore(packet) ||
		itemSize > this.dht.options.MaxItemSize ||
		itemSize+storageSize > this.dht.options.MaxStorageSize {

		this.Stored(packet, false)

		return
	}

	this.dht.Lock()
	this.dht.store[hex.EncodeToString(stInst.Hash)] = stInst.Data
	this.dht.Unlock()

	this.Stored(packet, true)
}

func (this *Node) Stored(packet Packet, hasStored bool) {
	this.dht.logger.Debug(this, "< STORED")

	data := this.newPacket(COMMAND_STORED, packet.Header.MessageHash, hasStored)

	this.send(data)
}

func (this *Node) OnStored(packet Packet, done CallbackChan) {
	this.dht.logger.Debug(this, "> STORED")

	done.c <- packet
}

func (this *Node) Custom(value interface{}) chan interface{} {
	this.dht.logger.Debug(this, "< CUSTOM")

	data := this.newPacket(COMMAND_CUSTOM, []byte{}, value)

	return this.send(data)
}

func (this *Node) OnCustom(packet Packet) {
	this.dht.logger.Debug(this, "> CUSTOM")

	res := this.dht.onCustomCmd(packet)
	this.dht.logger.Debug(this, "< CUSTOM ANSWER")

	if res == nil {
		this.send(this.newPacket(COMMAND_CUSTOM_ANSWER, packet.Header.MessageHash, "Unknown"))
		return
	}

	this.send(this.newPacket(COMMAND_CUSTOM_ANSWER, packet.Header.MessageHash, res))
}

func (this *Node) OnCustomAnswer(packet Packet, done CallbackChan) {
	this.dht.logger.Debug(this, "> CUSTOM ANSWER")

	done.c <- packet
}

func (this *Node) Broadcast(packet Packet) chan interface{} {
	if !this.dht.hasBroadcast(packet.Header.MessageHash) {
		this.dht.gotBroadcast = append(this.dht.gotBroadcast, packet.Header.MessageHash)
	}

	this.dht.logger.Debug(this, "< BROADCAST")
	// data := this.newPacket(COMMAND_BROADCAST, "", value)

	return this.send(packet)
}

func (this *Node) OnBroadcast(packet Packet) {
	if this.dht.hasBroadcast(packet.Header.MessageHash) {
		return
	}

	this.dht.gotBroadcast = append(this.dht.gotBroadcast, packet.Header.MessageHash)

	this.dht.logger.Debug(this, "> BROADCAST")

	this.dht.Broadcast(packet)
	this.dht.onBroadcast(packet)

	// this.send(this.newPacket(COMMAND_NOOP, packet.Header.MessageHash, nil))
}

type RepeatCmd struct {
	Hash  []byte
	Value []int
}

func (this *Node) RepeatPlease(hash []byte, value []int) chan interface{} {
	this.dht.logger.Debug(this, "< REPEAT PLEASE", len(value))

	data := this.newPacket(COMMAND_REPEAT_PLEASE, []byte{}, RepeatCmd{
		Hash:  hash,
		Value: value,
	})

	return this.send(data)
}

func (this *Node) OnRepeatPlease(packet Packet) {
	var repeatCmd RepeatCmd
	packet.GetData(&repeatCmd)

	missing := repeatCmd.Value

	this.dht.logger.Debug(this, "> REPEAT PLEASE")

	hash := repeatCmd.Hash

	this.dht.Lock()

	data, ok := this.dht.sentMsgs[string(hash)]

	if !ok {
		this.dht.Unlock()
		return
	}

	cmd, ok := this.dht.commandQueue[hex.EncodeToString(hash)]

	if !ok {
		this.dht.Unlock()
		return
	}

	cmd.timer.Stop()
	cmd.timer = time.NewTimer(time.Second)

	this.dht.logger.Debug(this, "< REPEATING")
	for _, id := range missing {
		_, err := this.dht.server.WriteTo(data[id], this.addr)

		if err != nil {
			this.dht.Unlock()
			return
		}
	}
	this.dht.Unlock()
}

type UDPWrapper struct {
	Id    int
	Total int
	Hash  []byte
	Data  []byte
}
type UDPWrapperList []UDPWrapper

func (a UDPWrapperList) Len() int           { return len(a) }
func (a UDPWrapperList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a UDPWrapperList) Less(i, j int) bool { return a[i].Id < a[j].Id }

func CreateUDPWrappers(dht *Dht, packet Packet) ([][]byte, error) {
	dht.RLock()
	msg, ok := dht.sentMsgs[string(packet.Header.MessageHash)]
	dht.RUnlock()

	if ok {
		return msg, nil
	}

	var blob bytes.Buffer
	enc := gob.NewEncoder(&blob)

	err := enc.Encode(packet)

	if err != nil {
		return [][]byte{}, err
	}

	buff := blob.Bytes()

	total := (len(buff) / (BUFFER_SIZE - 128)) + 1

	res := [][]byte{}

	i := 0
	for len(buff) > 0 {
		smalest := len(buff)

		if smalest > BUFFER_SIZE-128 {
			smalest = BUFFER_SIZE - 128
		}

		toSend := buff[:smalest]
		buff = buff[smalest:]

		wrapper := UDPWrapper{
			Id:    i,
			Total: total,
			Hash:  packet.Header.MessageHash,
			Data:  toSend,
		}

		i++

		var blob bytes.Buffer
		enc := gob.NewEncoder(&blob)

		err := enc.Encode(wrapper)

		if err != nil {
			return [][]byte{}, err
		}

		res = append(res, blob.Bytes())
	}

	dht.Lock()
	dht.sentMsgs[string(packet.Header.MessageHash)] = res
	dht.Unlock()

	return res, nil
}

func (this *Node) send(packet Packet) chan interface{} {
	// var blob bytes.Buffer
	// enc := gob.NewEncoder(&blob)

	// err := enc.Encode(packet)

	res := make(chan interface{})
	packets, err := CreateUDPWrappers(this.dht, packet)

	if err != nil {
		res <- errors.New("Error creating UDP Wrapper" + err.Error())

		return res
	}

	timer := time.NewTimer(time.Second)

	this.dht.Lock()
	this.dht.commandQueue[hex.EncodeToString(packet.Header.MessageHash)] = CallbackChan{
		timer: timer,
		c:     res,
	}

	for _, pack := range packets {
		_, err = this.dht.server.WriteTo(pack, this.addr)

		if err != nil {
			res <- errors.New("Error Writing" + err.Error())

			return res
		}
	}
	this.dht.Unlock()

	go func() {
		<-timer.C

		this.dht.Lock()
		delete(this.dht.commandQueue, hex.EncodeToString(packet.Header.MessageHash))
		this.dht.Unlock()

		var err string

		if len(this.contact.Hash) > 0 {
			err = hex.EncodeToString(this.contact.Hash[:16]) + " Timeout"
		} else {
			err = this.contact.Addr + " Timeout"
		}

		res <- errors.New(err)

		close(res)

		// this.disconnect()
	}()

	this.dht.Lock()
	defer this.dht.Unlock()
	return this.dht.commandQueue[hex.EncodeToString(packet.Header.MessageHash)].c
}

func (this *Node) disconnect() {
	// this.dht.Lock()
	// defer this.dht.Unlock()

	this.dht.routing.RemoveNode(this.contact)

	// for _, res := range this.dht.commandQueue {
	// 	res.timer.Stop()
	// 	// close(res.c)
	// }
}
