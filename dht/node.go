package dht

import (
	"bufio"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"sync"
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
)

type Callback func(val Packet, err error)

type CallbackChan struct {
	timer *time.Timer
	c     chan interface{}
}

type Node struct {
	sync.RWMutex
	contact      PacketContact
	lastSeen     int64
	socket       *bufio.ReadWriter
	commandQueue map[string]CallbackChan
	listening    bool
	dht          *Dht
}

type PacketContact struct {
	Hash []byte
	Addr string
}

type PacketHeader struct {
	DateSent    int64
	Command     int
	Sender      PacketContact
	ResponseTo  []byte
	MessageHash []byte
}

type Packet struct {
	Header PacketHeader
	Data   interface{}
}

type StoreInst struct {
	Hash []byte
	Data interface{}
}

type CustomCmd struct {
	Command int
	Data    interface{}
}

func (this *Node) newPacket(command int, responseTo []byte, data interface{}) Packet {
	packet := Packet{
		Header: PacketHeader{
			DateSent:    time.Now().UnixNano(),
			Command:     command,
			ResponseTo:  responseTo,
			MessageHash: []byte{},
			Sender: PacketContact{
				Addr: this.dht.options.ListenAddr,
				Hash: this.dht.hash,
			},
		},
		Data: data,
	}

	tmp, err := msgpack.Marshal(&packet)

	if err != nil {
		log.Fatal(err)
	}

	packet.Header.MessageHash = NewHash(tmp)

	return packet
}

func NewNode(dht *Dht, addr string, hash []byte) *Node {
	return &Node{
		dht:          dht,
		listening:    false,
		lastSeen:     time.Now().Unix(),
		commandQueue: make(map[string]CallbackChan),
		contact: PacketContact{
			Addr: addr,
			Hash: hash,
		},
	}
}

func NewNodeSocket(dht *Dht, addr string, hash []byte, socket *bufio.ReadWriter) *Node {
	node := NewNode(dht, addr, hash)

	node.socket = socket

	return node
}

func (this *Node) Redacted() interface{} {
	if len(this.contact.Hash) == 0 {
		return this.contact.Addr
	}

	return hex.EncodeToString(this.contact.Hash)[:16]
}

func (this *Node) Attach() error {
	this.loop()

	return nil
}

func (this *Node) Connect() error {
	if this.listening {
		return nil
	}

	if this.socket != nil {
		return this.Attach()
	}

	this.dht.logger.Debug(this, ". Connect")

	conn, err := net.Dial("tcp", this.contact.Addr)

	if err != nil {
		return err
	}

	this.socket = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	this.loop()

	promise := this.Ping()

	res := <-promise

	switch res.(type) {
	case error:
		return res.(error)
	default:
	}

	this.dht.logger.Debug(this, "o Connected")
	return nil
}

func (this *Node) loop() {
	go (func() {
		this.listening = true

		for {
			var packet Packet

			dec := gob.NewDecoder(this.socket)

			err := dec.Decode(&packet)

			if err != nil {
				if err.Error() == "EOF" {
					this.disconnect()

					return
				}

				this.dht.logger.Error(this, "x Error reading", err.Error())
			}

			if len(packet.Header.ResponseTo) > 0 {
				this.Lock()
				cb, ok := this.commandQueue[hex.EncodeToString(packet.Header.ResponseTo)]
				this.Unlock()

				if !ok {
					this.dht.logger.Error(this, "x Unknown response: ", packet.Header.ResponseTo, packet)
					continue
				}

				cb.timer.Stop()

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
					continue
				}

				close(cb.c)
				this.Lock()
				delete(this.commandQueue, hex.EncodeToString(packet.Header.ResponseTo))
				this.Unlock()
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
				default:
					this.dht.logger.Error(this, "x query: UNKNOWN COMMAND", packet.Header.Command)
					continue
				}
			}
		}
		this.listening = false
	})()
}

func (this *Node) Ping() chan interface{} {
	this.dht.logger.Debug(this, "< PING")

	return this.send(this.newPacket(COMMAND_PING, []byte{}, nil))
}

func (this *Node) OnPing(packet Packet) {
	this.dht.logger.Debug(this, "> PING")

	if len(this.contact.Hash) == 0 {
		this.contact.Addr = packet.Header.Sender.Addr
		this.contact.Hash = packet.Header.Sender.Hash

	}
	this.dht.routing.AddNode(this)

	this.Pong(packet.Header.MessageHash)
}

func (this *Node) Pong(responseTo []byte) chan interface{} {
	this.dht.logger.Debug(this, "< PONG")

	data := this.newPacket(COMMAND_PONG, responseTo, nil)

	return this.send(data)
}

func (this *Node) OnPong(packet Packet, cb CallbackChan) {
	this.dht.logger.Debug(this, "> PONG")

	if len(this.contact.Hash) == 0 {
		this.contact.Addr = packet.Header.Sender.Addr
		this.contact.Hash = packet.Header.Sender.Hash

	}
	this.dht.routing.AddNode(this)

	cb.c <- nil
}

func (this *Node) Fetch(hash []byte) chan interface{} {
	this.dht.logger.Debug(this, "< FETCH", hex.EncodeToString(hash)[:16])

	data := this.newPacket(COMMAND_FETCH, []byte{}, hash)

	return this.send(data)
}

func (this *Node) OnFetch(packet Packet) {
	this.dht.logger.Debug(this, "> FETCH", hex.EncodeToString(packet.Data.([]byte))[:16])

	val, ok := this.dht.store[hex.EncodeToString(packet.Data.([]byte))]

	if ok {
		this.Found(packet, val)
		return
	}

	this.OnFetchNodes(packet)
}

func (this *Node) FetchNodes(hash []byte) chan interface{} {
	this.dht.logger.Debug(this, "< FETCH NODES", hex.EncodeToString(hash)[:16])

	data := this.newPacket(COMMAND_FETCH_NODES, []byte{}, hash)

	return this.send(data)
}

func (this *Node) OnFetchNodes(packet Packet) {
	this.dht.logger.Debug(this, "> FETCH NODES", hex.EncodeToString(packet.Data.([]byte))[:16])

	bucket := this.dht.routing.FindNode(packet.Data.([]byte))

	var nodesContact []PacketContact

	for _, node := range bucket {
		nodesContact = append(nodesContact, node.contact)
	}

	this.FoundNodes(packet, nodesContact)
}

func (this *Node) FoundNodes(packet Packet, nodesContact []PacketContact) {
	this.dht.logger.Debug(this, "< FOUND NODES", len(nodesContact))

	data := this.newPacket(COMMAND_FOUND_NODES, packet.Header.MessageHash, nodesContact)

	this.send(data)
}

func (this *Node) OnFoundNodes(packet Packet, done CallbackChan) {
	this.dht.logger.Debug(this, "> FOUND NODES", len(packet.Data.([]PacketContact)))

	done.c <- packet
}

func (this *Node) Found(packet Packet, value interface{}) {
	this.dht.logger.Debug(this, "< FOUND", value)

	data := this.newPacket(COMMAND_FOUND, packet.Header.MessageHash, value)

	this.send(data)
}

func (this *Node) OnFound(packet Packet, done CallbackChan) {
	this.dht.logger.Debug(this, "> FOUND", packet.Data)

	done.c <- packet
}

func (this *Node) Store(hash []byte, value interface{}) chan interface{} {
	this.dht.logger.Debug(this, "< STORE", hex.EncodeToString(hash)[:16], value)

	data := this.newPacket(COMMAND_STORE, []byte{}, StoreInst{Hash: hash, Data: value})

	return this.send(data)
}

func (this *Node) OnStore(packet Packet) {
	this.dht.logger.Debug(this, "> STORE", packet.Data.(StoreInst).Hash, packet.Data.(StoreInst).Data)

	this.dht.store[hex.EncodeToString(packet.Data.(StoreInst).Hash)] = packet.Data.(StoreInst).Data

	this.Stored(packet)
}

func (this *Node) Stored(packet Packet) {
	this.dht.logger.Debug(this, "< STORED")

	data := this.newPacket(COMMAND_STORED, packet.Header.MessageHash, nil)

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

	res := this.dht.OnCustomCmd(packet)
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
	this.dht.OnBroadcast(packet)

	// this.send(this.newPacket(COMMAND_NOOP, packet.Header.MessageHash, nil))
}

func (this *Node) send(packet Packet) chan interface{} {
	this.Lock()
	defer this.Unlock()

	enc := gob.NewEncoder(this.socket)

	err := enc.Encode(packet)

	res := make(chan interface{})

	if err != nil {
		res <- errors.New("Error Encode" + err.Error())

		return res
	}

	timer := time.NewTimer(time.Second * 5)

	this.commandQueue[hex.EncodeToString(packet.Header.MessageHash)] = CallbackChan{
		timer: timer,
		c:     res,
	}

	go func() {
		<-timer.C

		this.Lock()
		delete(this.commandQueue, hex.EncodeToString(packet.Header.MessageHash))
		this.Unlock()

		var err string

		if len(this.contact.Hash) > 0 {
			err = hex.EncodeToString(this.contact.Hash[:16]) + " Timeout"
		} else {
			err = this.contact.Addr + " Timeout"
		}

		res <- errors.New(err)

		close(res)
	}()

	this.socket.Flush()

	return this.commandQueue[hex.EncodeToString(packet.Header.MessageHash)].c
}

func (this *Node) disconnect() {
	this.dht.routing.RemoveNode(this)
}
