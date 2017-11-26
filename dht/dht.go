package dht

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	logging "github.com/op/go-logging"
)

type Dht struct {
	sync.RWMutex
	routing      *Routing
	options      DhtOptions
	hash         []byte
	running      bool
	store        map[string]interface{}
	commandQueue map[string]CallbackChan
	logger       *logging.Logger
	server       net.PacketConn
	gotBroadcast [][]byte
}

type DhtOptions struct {
	NoRepublishOnExit bool
	ListenAddr        string
	BootstrapAddr     string
	Verbose           int
	Cluster           int
	Stats             bool
	Interactif        bool
	OnStore           func(Packet) bool
	OnCustomCmd       func(Packet) interface{}
	OnBroadcast       func(Packet) interface{}
}

func New(options DhtOptions) *Dht {
	res := &Dht{
		routing:      NewRouting(),
		options:      options,
		running:      false,
		store:        make(map[string]interface{}),
		commandQueue: make(map[string]CallbackChan),
		logger:       logging.MustGetLogger("dht"),
	}

	gob.Register([]PacketContact{})
	gob.Register(StoreInst{})
	gob.Register(CustomCmd{})

	initLogger(res)

	res.routing.dht = res

	res.logger.Debug("DHT version 0.0.1")

	r := rand.Intn(60) - 60
	timer := time.NewTicker(time.Minute*10 + (time.Second * time.Duration(r)))

	go func() {
		for range timer.C {
			res.republish()
		}
	}()

	return res
}

func initLogger(dht *Dht) {
	var format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)

	backend := logging.NewLogBackend(os.Stderr, "", 0)

	var logLevel logging.Level
	switch dht.options.Verbose {
	case 0:
		logLevel = logging.CRITICAL
	case 1:
		logLevel = logging.ERROR
	case 2:
		logLevel = logging.WARNING
	case 3:
		logLevel = logging.NOTICE
	case 4:
		logLevel = logging.INFO
	case 5:
		logLevel = logging.DEBUG
	default:
		logLevel = 2
	}

	backendFormatter := logging.NewBackendFormatter(backend, format)

	backendLeveled := logging.AddModuleLevel(backendFormatter)

	backendLeveled.SetLevel(logLevel, "")

	logging.SetBackend(backendLeveled)
}

func (this *Dht) republish() {
	for k, v := range this.store {
		h, _ := hex.DecodeString(k)
		this.StoreAt(h, v)
	}

	this.logger.Debug("Republished", len(this.store))
}

func (this *Dht) Store(value interface{}) ([]byte, int, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(value)

	if err != nil {
		return []byte{}, 0, err
	}

	hash := NewHash(buf.Bytes())

	return this.StoreAt(hash, value)
}

func (this *Dht) StoreAt(hash []byte, value interface{}) ([]byte, int, error) {
	bucket := this.fetchNodes(hash)

	if len(bucket) == 0 {
		return []byte{}, 0, errors.New("No nodes found")
	}

	fn := func(node *Node) chan interface{} {
		return node.Store(hash, value)
	}

	query := NewQuery(hash, fn, this)

	res := query.Run().([]bool)

	if len(res) == 0 {
		return []byte{}, 0, errors.New("No answers from nodes")
	}

	storedOkNb := 0
	for _, stored := range res {
		if stored {
			storedOkNb++
		}
	}

	if storedOkNb == 0 {
		return []byte{}, 0, errors.New(hex.EncodeToString(hash) + ": The key might be existing already")
	}

	return hash, storedOkNb, nil
}

func (this *Dht) Fetch(hash []byte) (interface{}, error) {
	fn := func(node *Node) chan interface{} {
		return node.Fetch(hash)
	}

	query := NewQuery(hash, fn, this)

	res := query.Run()

	switch res.(type) {
	case []*Node:
		return nil, errors.New("Not found")
	default:
	}

	return res, nil
}

func (this *Dht) fetchNodes(hash []byte) []*Node {
	fn := func(node *Node) chan interface{} {
		return node.FetchNodes(hash)
	}

	query := NewQuery(hash, fn, this)

	return query.Run().([]*Node)
}

func (this *Dht) bootstrap() error {
	this.logger.Debug("Connecting to bootstrap node", this.options.BootstrapAddr)

	addr, err := net.ResolveUDPAddr("udp", this.options.BootstrapAddr)

	if err != nil {
		return err
	}

	bootstrapNode := NewNode(this, addr, []byte{})

	// this.routing.AddNode(bootstrapNode)

	err, ok := (<-bootstrapNode.Ping()).(error)

	if ok {
		return err
	}

	_ = this.fetchNodes(this.hash)

	for i, bucket := range this.routing.buckets {
		if len(bucket) != 0 {
			continue
		}

		h := NewRandomHash()
		h = this.routing.nCopy(h, this.hash, i)

		_ = this.fetchNodes(h)
	}

	this.logger.Info("Ready...")

	if this.options.Interactif {
		go this.Cli()
	}

	return nil
}

func (this *Dht) Start() error {
	if this.running {
		return errors.New("Already started")
	}

	this.hash = NewRandomHash()

	this.logger.Debug("Own hash", hex.EncodeToString(this.hash))

	l, err := net.ListenPacket("udp", this.options.ListenAddr)

	if err != nil {
		return errors.New("Error listening:" + err.Error())
	}

	this.server = l

	go func() {
		this.logger.Info("Listening on " + this.options.ListenAddr)

		if err := this.loop(); err != nil {
			this.running = false
			this.logger.Error("Main loop: " + err.Error())
		}
	}()

	this.running = true

	if len(this.options.BootstrapAddr) > 0 {
		if err := this.bootstrap(); err != nil {
			this.Stop()
			return errors.New("Bootstrap: " + err.Error())
		}
	} else {
		if this.options.Interactif {
			go this.Cli()
		}
	}

	return nil
}

func (this *Dht) loop() error {
	this.running = true

	defer this.server.Close()

	for this.running {
		var packet [1024 * 8]byte

		n, addr, err := this.server.ReadFrom(packet[0:])

		if err != nil {
			if this.running == false {
				return nil
			}

			return errors.New("Error reading:" + err.Error())
		}

		go this.handleInPacket(addr, packet[0:n])
	}

	return nil
}

func (this *Dht) Stop() {
	if !this.running {
		return
	}

	if !this.options.NoRepublishOnExit {
		this.republish()
	}

	this.running = false

	this.server.Close()

}

func (this *Dht) handleInPacket(addr net.Addr, blob_ []byte) {
	var packet Packet

	var blob bytes.Buffer
	blob.Write(blob_)

	dec := gob.NewDecoder(&blob)

	err := dec.Decode(&packet)

	if err != nil {
		this.logger.Warning("Invalid packet")

		return
	}

	var node *Node
	addr, err = net.ResolveUDPAddr("udp", packet.Header.Sender.Addr)

	node = NewNodeContact(this, addr, packet.Header.Sender)

	this.routing.AddNode(packet.Header.Sender)

	node.HandleInPacket(packet)
}

func (this *Dht) Logger() *logging.Logger {
	return this.logger
}

func (this *Dht) CustomCmd(data interface{}) {
	bucket := this.routing.FindNode(this.hash)

	for _, contact := range bucket {
		addr, _ := net.ResolveUDPAddr("udp", contact.Addr)

		node := NewNodeContact(this, addr, contact)
		<-node.Custom(data)
	}
}

func (this *Dht) hasBroadcast(hash []byte) bool {
	for _, h := range this.gotBroadcast {
		if compare(h, hash) == 0 {
			return true
		}
	}

	return false
}

func compare(hash1, hash2 []byte) int {
	if len(hash1) != len(hash2) {
		return len(hash1) - len(hash2)
	}

	for i, v := range hash1 {
		if v != hash2[i] {
			return int(v) - int(hash2[i])
		}
	}

	return 0
}

func (this *Dht) Broadcast(data interface{}) {
	bucket := this.routing.FindNode(this.hash)

	var packet Packet
	switch data.(type) {
	case Packet:
		packet = data.(Packet)
	default:
		packet = NewPacket(this, COMMAND_BROADCAST, []byte{}, data)
	}

	for _, contact := range bucket {
		addr, _ := net.ResolveUDPAddr("udp", contact.Addr)

		node := NewNodeContact(this, addr, contact)
		node.Broadcast(packet)
	}
}

func (this *Dht) Running() bool {
	return this.running
}

func (this *Dht) Wait() {
	for this.running {
		time.Sleep(time.Second)
	}
}

func (this *Dht) onCustomCmd(packet Packet) interface{} {
	if this.options.OnCustomCmd != nil {
		return this.options.OnCustomCmd(packet)
	}

	return nil
}

func (this *Dht) onBroadcast(packet Packet) interface{} {
	if this.options.OnBroadcast != nil {
		return this.options.OnBroadcast(packet)
	}

	return nil
}

func (this *Dht) onStore(packet Packet) bool {
	if this.options.OnStore != nil {
		return this.options.OnStore(packet)
	}

	return true
}

func (this *Dht) GetConnectedNumber() int {
	return this.routing.Size()
}

func (this *Dht) StoredKeys() int {
	return len(this.store)
}
