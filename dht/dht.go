package dht

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"math/rand"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	logging "github.com/op/go-logging"
)

type Dht struct {
	sync.RWMutex
	routing       *Routing
	options       DhtOptions
	hash          []byte
	running       bool
	store         map[string][]byte
	commandQueue  map[string]CallbackChan
	logger        *logging.Logger
	server        net.PacketConn
	gotBroadcast  [][]byte
	messageChunks map[string]WaitingPartMsg
	sentMsgs      map[string][][]byte
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
	MaxStorageSize    int
	MaxItemSize       int
}

type WaitingPartMsg struct {
	timer   *time.Timer
	timeout *time.Timer
	hash    []byte
	addr    net.Addr
	total   int
	parts   []UDPWrapper
}

func New(options DhtOptions) *Dht {
	if options.MaxStorageSize == 0 {
		options.MaxStorageSize = 500000000 // ~500Mo
	}

	if options.MaxItemSize == 0 {
		options.MaxItemSize = 5000000 // ~5Mo
	}

	res := &Dht{
		routing:       NewRouting(),
		options:       options,
		running:       false,
		store:         make(map[string][]byte),
		commandQueue:  make(map[string]CallbackChan),
		messageChunks: make(map[string]WaitingPartMsg),
		sentMsgs:      make(map[string][][]byte),
		logger:        logging.MustGetLogger("dht"),
	}

	gob.Register([]PacketContact{})
	gob.Register(StoreInst{})
	gob.Register(CustomCmd{})
	gob.Register(UDPWrapper{})
	gob.Register(RepeatCmd{})

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
	var blob bytes.Buffer
	enc := gob.NewEncoder(&blob)
	err := enc.Encode(value)

	if err != nil {
		return []byte{}, 0, errors.New("Store: cannot marshal value")
	}

	if blob.Len() > this.options.MaxItemSize {
		return []byte{}, 0, errors.New("Store: Exceeds max limit")
	}

	bucket := this.fetchNodes(hash)

	if len(bucket) == 0 {
		return []byte{}, 0, errors.New("No nodes found")
	}

	data := blob.Bytes()
	fn := func(node *Node) chan interface{} {
		return node.Store(hash, data)
	}

	query := NewQuery(hash, fn, this)

	res, ok := query.Run().([]bool)

	if !ok || len(res) == 0 {
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

func (this *Dht) Fetch(hash []byte, ptr interface{}) error {
	fn := func(node *Node) chan interface{} {
		return node.Fetch(hash)
	}

	query := NewQuery(hash, fn, this)

	res := query.Run()

	switch res.(type) {
	case []*Node:
		return errors.New("Not found")
	case Packet:
		packet := res.(Packet)

		// var data []byte
		return packet.GetData(ptr)

		// var blob bytes.Buffer
		// blob.Write(data)

		// dec := gob.NewDecoder(&blob)
		// return dec.Decode(ptr)

		// return nil
	default:
	}

	// var blob bytes.Buffer
	// blob.Write(res.([]byte))

	// dec := gob.NewDecoder(&blob)

	// if err := dec.Decode(ptr); err != nil {
	// 	return err
	// }

	return errors.New("Unknown fetched data")
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

const BUFFER_SIZE = 1024 * 4

func (this *Dht) loop() error {
	this.running = true

	defer this.server.Close()

	// msgs := make(map[string][]UDPWrapper)

	for this.running {
		var message [BUFFER_SIZE]byte

		_, addr, err := this.server.ReadFrom(message[0:])

		if err != nil {
			if this.running == false {
				return nil
			}

			return errors.New("Error reading:" + err.Error())
		}

		go this.handleInWrapper(addr, message[0:])
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

func (this *Dht) handleInWrapper(addr net.Addr, blob_ []byte) {
	var wrapper UDPWrapper

	var blob bytes.Buffer
	blob.Write(blob_)

	dec := gob.NewDecoder(&blob)

	err := dec.Decode(&wrapper)

	if err != nil {
		this.logger.Warning("Invalid packet wrapper")

		return
	}

	this.Lock()

	curMsg, ok := this.messageChunks[string(wrapper.Hash)]

	if !ok {
		curMsg = WaitingPartMsg{
			timer:   time.NewTimer(time.Millisecond * 100),
			timeout: time.NewTimer(time.Second * 30),
			hash:    wrapper.Hash,
			total:   wrapper.Total,
			addr:    addr,
			parts:   []UDPWrapper{},
		}

		go func() {
			<-curMsg.timeout.C
			curMsg.timeout = nil
			curMsg.timer.Stop()
			curMsg.timer = nil
			this.Lock()
			delete(this.messageChunks, string(wrapper.Hash))
			this.Unlock()
		}()

		var onTimer func()
		onTimer = func() {
			if curMsg.timeout == nil {
				return
			}

			<-curMsg.timer.C

			this.RLock()
			if len(this.messageChunks[string(wrapper.Hash)].parts) < this.messageChunks[string(wrapper.Hash)].total {
				this.RUnlock()
				this.onMsgTimeout(this.messageChunks[string(wrapper.Hash)])
			} else {
				this.RUnlock()
			}

			curMsg.timer = time.NewTimer(time.Millisecond * 150)
			go onTimer()
		}

		go onTimer()
	}

	curMsg.parts = append(curMsg.parts, wrapper)

	if len(curMsg.parts)*BUFFER_SIZE > this.options.MaxItemSize {
		curMsg.timeout.Stop()
		curMsg.timer.Stop()
		delete(this.messageChunks, string(wrapper.Hash))
		this.Unlock()

		return
	}

	this.messageChunks[string(wrapper.Hash)] = curMsg
	this.Unlock()

	if len(curMsg.parts) == wrapper.Total {
		curMsg.timer.Stop()
		sort.Sort(UDPWrapperList(curMsg.parts))
		res := []byte{}
		for _, w := range curMsg.parts {
			res = append(res, w.Data...)
		}

		this.Lock()
		delete(this.messageChunks, string(wrapper.Hash))
		this.Unlock()

		this.handleInPacket(addr, res)
	}
}

func (this *Dht) onMsgTimeout(msg WaitingPartMsg) {
	node := NewNode(this, msg.addr, []byte{})

	missing := []int{}
	sort.Sort(UDPWrapperList(msg.parts))

	j := 0
	for i := 0; i < msg.total; i++ {
		if j >= len(msg.parts) || msg.parts[j].Id != i {
			missing = append(missing, i)
		} else {
			j++
		}
	}

	node.RepeatPlease(msg.hash, missing)
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

func (this *Dht) StorageSize() int {
	this.RLock()
	defer this.RUnlock()

	size := 0
	for k, v := range this.store {
		size += len(k) + len(v)
	}

	return size
}
