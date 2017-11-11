package dht

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack"
)

const (
	COMMAND_PING = iota
	COMMAND_PONG
	COMMAND_STORE
	COMMAND_STORED
	COMMAND_FETCH
	COMMAND_FETCH_NODES
	COMMAND_FOUND
	COMMAND_FOUND_NODES
)

type Callback func(val Packet, err error)

type Node struct {
	sync.RWMutex
	contact      PacketContact
	lastSeen     int64
	socket       *bufio.ReadWriter
	commandQueue map[string]Callback
	listening    bool
	dht          *Dht
}

type PacketContact struct {
	Hash string
	Addr string
}

type PacketHeader struct {
	DateSent    int64
	Command     int
	Sender      PacketContact
	ResponseTo  string
	MessageHash string
}

type Packet struct {
	Header PacketHeader
	Data   interface{}
}

type StoreInst struct {
	Hash string
	Data interface{}
}

func (this *Node) newPacket(command int, responseTo string, data interface{}) Packet {
	packet := Packet{
		Header: PacketHeader{
			DateSent:    time.Now().UnixNano(),
			Command:     command,
			ResponseTo:  responseTo,
			MessageHash: "",
			Sender: PacketContact{
				Addr: this.dht.options.ListenAddr,
				Hash: this.dht.hash,
			},
		},
		Data: data,
	}

	gob.Register([]PacketContact{})
	gob.Register(StoreInst{})

	tmp, err := msgpack.Marshal(&packet)

	if err != nil {
		log.Fatal(err)
	}

	packet.Header.MessageHash = NewHash(tmp)

	return packet
}

func NewNode(dht *Dht, addr string, hash string) *Node {
	return &Node{
		dht:          dht,
		listening:    false,
		lastSeen:     time.Now().Unix(),
		commandQueue: make(map[string]Callback),
		contact: PacketContact{
			Addr: addr,
			Hash: hash,
		},
	}
}

func NewNodeSocket(dht *Dht, addr string, hash string, socket *bufio.ReadWriter) *Node {
	node := NewNode(dht, addr, hash)

	node.socket = socket

	return node
}

func (this *Node) Redacted() interface{} {
	if len(this.contact.Hash) == 0 {
		return this.contact.Addr
	}

	return this.contact.Hash[:16]
}

func (this *Node) Connect(done Callback) {
	this.dht.logger.Debug(this, ". Connect")

	if this.listening {
		this.dht.logger.Debug(this, "x Already listening...")
		done(Packet{}, nil)

		return
	}

	if this.socket != nil {
		this.dht.logger.Debug(this, "x Is already connected...")
		this.loop()

		done(Packet{}, nil)
		return
	}

	conn, err := net.Dial("tcp", this.contact.Addr)

	if err != nil {
		done(Packet{}, err)

		return
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	this.socket = rw

	this.loop()

	this.Ping(func(val Packet, err error) {
		if err != nil {
			done(Packet{}, err)

			return
		}

		this.dht.logger.Debug(this, "o Connected")
		done(val, nil)
	})
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
				cb, ok := this.commandQueue[packet.Header.ResponseTo]
				this.Unlock()

				if !ok {
					this.dht.logger.Error(this, "x Unknown response: ", packet.Header.ResponseTo)
					continue
				}

				switch packet.Header.Command {
				case COMMAND_PONG:
					this.OnPong(packet, cb)
				case COMMAND_FOUND:
					this.OnFound(packet, cb)
				case COMMAND_FOUND_NODES:
					this.OnFoundNodes(packet, cb)
				case COMMAND_STORED:
					this.OnStored(packet, cb)

				default:
					this.dht.logger.Error(this, "x answer: UNKNOWN COMMAND", packet.Header.Command)
					continue
				}

				this.Lock()
				delete(this.commandQueue, packet.Header.ResponseTo)
				this.Unlock()
			} else {
				switch packet.Header.Command {
				case COMMAND_PING:
					this.OnPing(packet)
				case COMMAND_FETCH:
					this.OnFetch(packet)
				case COMMAND_FETCH_NODES:
					this.OnFetchNodes(packet)
				case COMMAND_STORE:
					this.OnStore(packet)
				default:
					this.dht.logger.Error(this, "x query: UNKNOWN COMMAND", packet.Header.Command)
					continue
				}
			}
		}
		this.listening = false
	})()
}

func (this *Node) Ping(done Callback) {
	this.dht.logger.Debug(this, "< PING")

	this.send(this.newPacket(COMMAND_PING, "", nil), done)
}

func (this *Node) OnPing(packet Packet) {
	if len(this.contact.Hash) == 0 {
		this.contact.Addr = packet.Header.Sender.Addr
		this.contact.Hash = packet.Header.Sender.Hash

	}
	this.dht.routing.AddNode(this)

	this.dht.logger.Debug(this, "> PING")

	this.Pong(packet.Header.MessageHash)
}

func (this *Node) Pong(responseTo string) {
	this.dht.logger.Debug(this, "< PONG")

	data := this.newPacket(COMMAND_PONG, responseTo, nil)

	this.send(data, func(Packet, error) {})
}

func (this *Node) OnPong(packet Packet, done Callback) {
	this.dht.logger.Debug(this, "> PONG")

	if len(this.contact.Hash) == 0 {
		this.contact.Addr = packet.Header.Sender.Addr
		this.contact.Hash = packet.Header.Sender.Hash

	}
	this.dht.routing.AddNode(this)

	done(packet, nil)
}

func (this *Node) Fetch(hash string, done Callback) {
	this.dht.logger.Debug(this, "< FETCH", hash[:16])

	data := this.newPacket(COMMAND_FETCH, "", hash)

	this.send(data, done)
}

func (this *Node) OnFetch(packet Packet) {
	this.dht.logger.Debug(this, "> FETCH", packet.Data.(string)[:16])

	val, ok := this.dht.store[packet.Data.(string)]

	if ok {
		this.Found(packet, val)
		return
	}

	this.OnFetchNodes(packet)
}

func (this *Node) FetchNodes(hash string, done Callback) {
	this.dht.logger.Debug(this, "< FETCH NODES", hash[:16])

	data := this.newPacket(COMMAND_FETCH_NODES, "", hash)

	this.send(data, done)
}

func (this *Node) OnFetchNodes(packet Packet) {
	this.dht.logger.Debug(this, "> FETCH NODES", packet.Data.(string)[:16])

	bucket := this.dht.routing.FindNode(packet.Data.(string))

	var nodesContact []PacketContact

	for _, node := range bucket {
		nodesContact = append(nodesContact, node.contact)
	}

	this.FoundNodes(packet, nodesContact)
}

func (this *Node) FoundNodes(packet Packet, nodesContact []PacketContact) {
	this.dht.logger.Debug(this, "< FOUND NODES", len(nodesContact))

	data := this.newPacket(COMMAND_FOUND_NODES, packet.Header.MessageHash, nodesContact)

	this.send(data, func(Packet, error) {})
}

func (this *Node) OnFoundNodes(packet Packet, done Callback) {
	this.dht.logger.Debug(this, "> FOUND NODES", len(packet.Data.([]PacketContact)))

	done(packet, nil)
}

func (this *Node) Found(packet Packet, value interface{}) {
	this.dht.logger.Debug(this, "< FOUND", value)

	data := this.newPacket(COMMAND_FOUND, packet.Header.MessageHash, value)

	this.send(data, func(Packet, error) {})
}

func (this *Node) OnFound(packet Packet, done Callback) {
	this.dht.logger.Debug(this, "> FOUND", packet.Data)

	done(packet, nil)
}

func (this *Node) Store(hash string, value interface{}, done Callback) {
	this.dht.logger.Debug(this, "< STORE", hash[:16], value)

	data := this.newPacket(COMMAND_STORE, "", StoreInst{Hash: hash, Data: value})

	this.send(data, done)
}

func (this *Node) OnStore(packet Packet) {
	this.dht.logger.Debug(this, "> STORE", packet.Data.(StoreInst).Hash, packet.Data.(StoreInst).Data)

	// todo: check if best eligible
	this.dht.store[packet.Data.(StoreInst).Hash] = packet.Data.(StoreInst).Data

	this.Stored(packet)
}

func (this *Node) Stored(packet Packet) {
	this.dht.logger.Debug(this, "< STORED")

	data := this.newPacket(COMMAND_STORED, packet.Header.MessageHash, nil)

	this.send(data, func(Packet, error) {})
}

func (this *Node) OnStored(packet Packet, done Callback) {
	this.dht.logger.Debug(this, "> STORED")

	done(packet, nil)
}

func (this *Node) send(packet Packet, done Callback) {
	enc := gob.NewEncoder(this.socket)

	err := enc.Encode(packet)

	if err != nil {
		fmt.Println("Error Encode", err.Error())
	}

	// If you just wanted to wait, you could have used
	// `time.Sleep`. One reason a timer may be useful is
	// that you can cancel the timer before it expires.
	// Here's an example of that.

	timer := time.NewTimer(time.Second * 5)

	this.Lock()
	this.commandQueue[packet.Header.MessageHash] = func(v Packet, err error) {
		timer.Stop()

		done(v, err)
	}
	this.Unlock()

	go func() {
		<-timer.C
		done(packet, errors.New(this.contact.Hash[:16]+" Timeout"))

		this.Lock()
		delete(this.commandQueue, packet.Header.MessageHash)
		this.Unlock()
	}()

	this.socket.Flush()
}

func (this *Node) disconnect() {
	this.dht.routing.RemoveNode(this)
}
