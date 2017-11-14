package dht

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	logging "github.com/op/go-logging"
)

type Dht struct {
	routing *Routing
	options DhtOptions
	hash    string
	running bool
	store   map[string]interface{}
	logger  *logging.Logger
	server  net.Listener
}

type DhtOptions struct {
	ListenAddr    string
	BootstrapAddr string
	Verbose       int
	Stats         bool
	Interactif    bool
	OnStore       func()
	OnCustomCmd   func(Packet)
}

func New(options DhtOptions) *Dht {
	res := &Dht{
		routing: NewRouting(),
		options: options,
		running: false,
		store:   make(map[string]interface{}),
		logger:  logging.MustGetLogger("dht"),
	}

	initLogger(res)

	res.routing.dht = res

	res.logger.Debug("DHT version 0.0.1")

	timer := time.NewTicker(time.Minute * 30)

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
	for _, v := range this.store {
		this.Store(v)
	}
	this.logger.Debug("Republished", len(this.store))
}

func (this *Dht) Store(value interface{}) (string, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(value)

	if err != nil {
		return "", err
	}

	hash := NewHash(buf.Bytes())

	return this.StoreAt(hash, value)
}

func (this *Dht) StoreAt(hash string, value interface{}) (string, error) {
	fn := func(node *Node) chan interface{} {
		return node.Store(hash, value)
	}

	_, _, err := this.fetchNodes(hash, fn)

	if err != nil {
		return "", err
	}

	return hash, nil
}

func (this *Dht) Fetch(hash string) (interface{}, error) {
	// bucket := this.routing.FindNode(hash)

	fn := func(node *Node) chan interface{} {
		return node.Fetch(hash)
	}

	_, res, err := this.fetchNodes(hash, fn)

	if err != nil {
		return nil, err
	}

	if res == nil {
		return nil, errors.New("Not found")
	}

	return res, nil
}

func (this *Dht) processFoundBucket(hash string, foundNodes []PacketContact, best []*Node, blacklist []*Node, nodeFn func(*Node) chan interface{}) ([]*Node, interface{}, error) {
	if len(foundNodes) == 0 {
		return this.performConnectToAll(hash, foundNodes, best, blacklist, nodeFn)
	}

	foundNodes = this.filter(foundNodes, func(n PacketContact) bool {
		return !this.contains(blacklist, n)
	})

	foundNodes = this.filter(foundNodes, func(n PacketContact) bool {
		return !this.contains(best, n)
	})

	foundNodes = this.filter(foundNodes, func(n PacketContact) bool {
		return n.Hash != this.hash
	})

	if len(foundNodes) == 0 {
		return best, nil, nil
	}

	lowestDistanceInBest := -1

	for _, n := range best {
		dist := this.routing.distanceBetwin(hash, n.contact.Hash)

		if dist < lowestDistanceInBest {
			lowestDistanceInBest = dist
		}
	}

	// if lowestDistanceInBest > -1 {
	// 	foundNodes = this.filter(foundNodes, func(n PacketContact) bool {
	// 		return this.routing.distanceBetwin(hash, n.Hash) <= lowestDistanceInBest
	// 	})
	// }

	if len(foundNodes) == 0 {
		return best, nil, nil
	}

	return this.performConnectToAll(hash, foundNodes, best, blacklist, nodeFn)
}

func (this *Dht) fetchNodes(hash string, nodeFn func(*Node) chan interface{}) ([]*Node, interface{}, error) {
	return this.fetchNodes_(hash, this.routing.FindNode(hash), []*Node{}, []*Node{}, nodeFn)
}

func (this *Dht) fetchNodes_(hash string, bucket []*Node, best []*Node, blacklist []*Node, nodeFn func(*Node) chan interface{}) ([]*Node, interface{}, error) {
	var foundNodes []PacketContact

	if len(bucket) == 0 {
		return best, nil, nil
	}

	var wg sync.WaitGroup
	var resArr []interface{}
	var mutex sync.RWMutex

	for _, node := range bucket {
		wg.Add(1)
		test := node
		go func() {
			res := <-nodeFn(test)
			mutex.Lock()
			resArr = append(resArr, res)
			mutex.Unlock()
			wg.Done()
		}()
	}

	wg.Wait()

	for _, res := range resArr {
		switch res.(type) {
		case error:
			// return []*Node{}, nil, res.(error)
			continue
		case Packet:
			if res.(Packet).Header.Command == COMMAND_FOUND {
				return []*Node{}, res.(Packet).Data, nil
			}
			switch res.(Packet).Data.(type) {
			case []PacketContact:
				toAdd := res.(Packet).Data.([]PacketContact)
				for _, contact := range toAdd {
					if !this.contactContains(foundNodes, contact) {
						foundNodes = append(foundNodes, contact)
					}
				}
			}
		default:
		}
	}

	return this.processFoundBucket(hash, foundNodes, best, blacklist, nodeFn)
}

func (this *Dht) contains(bucket []*Node, node PacketContact) bool {
	for _, n := range bucket {
		if node.Hash == n.contact.Hash {
			return true
		}
	}

	return false
}

func (this *Dht) contactContains(bucket []PacketContact, node PacketContact) bool {
	for _, n := range bucket {
		if node.Hash == n.Hash {
			return true
		}
	}

	return false
}

func (this *Dht) filter(bucket []PacketContact, fn func(PacketContact) bool) []PacketContact {
	var res []PacketContact

	for _, node := range bucket {
		if fn(node) {
			res = append(res, node)
		}
	}

	return res
}

func (this *Dht) performConnectToAll(hash string, foundNodes []PacketContact, best []*Node, blacklist []*Node, nodeFn func(*Node) chan interface{}) ([]*Node, interface{}, error) {
	connectedNodes, err := this.connectToAll(foundNodes)

	if err != nil {
		return []*Node{}, nil, err
	}

	blacklist = append(blacklist, connectedNodes...)

	smalest := len(connectedNodes)

	if smalest > BUCKET_SIZE {
		smalest = BUCKET_SIZE
	}

	best = append(best, connectedNodes...)[:smalest]

	return this.fetchNodes_(hash, connectedNodes, best, blacklist, nodeFn)
}

func (this *Dht) connectToAll(foundNodes []PacketContact) ([]*Node, error) {
	newBucket := []*Node{}

	for _, foundNode := range foundNodes {
		n := this.routing.GetNode(foundNode.Hash)

		if n == nil {
			n = NewNode(this, foundNode.Addr, foundNode.Hash)
		}

		newBucket = append(newBucket, n)
	}

	return this.connectBucketAsync(newBucket)
}

func (this *Dht) connectBucketAsync(bucket []*Node) ([]*Node, error) {
	var res []*Node

	answers := 0

	if len(bucket) == 0 {
		return res, nil
	}

	for _, node := range bucket {
		err := node.Connect()
		answers++

		if err != nil {
			return nil, err
		}

		res = append(res, node)

		if answers == len(bucket) {
			return res, nil
		}
	}

	return []*Node{}, nil
}

func (this *Dht) bootstrap() error {
	this.logger.Debug("Connecting to bootstrap node", this.options.BootstrapAddr)

	bootstrapNode := NewNode(this, this.options.BootstrapAddr, "")

	if err := bootstrapNode.Connect(); err != nil {
		return err
	}

	fn := func(node *Node) chan interface{} {
		return node.FetchNodes(this.hash)
	}

	_, _, err := this.fetchNodes(this.hash, fn)

	if err != nil {
		return err
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

	this.logger.Debug("Own hash", this.hash)

	l, err := net.Listen("tcp", this.options.ListenAddr)

	if err != nil {
		return errors.New("Error listening:" + err.Error())
	}

	this.server = l

	this.logger.Info("Listening on " + this.options.ListenAddr)

	this.running = true

	if len(this.options.BootstrapAddr) > 0 {
		if err := this.bootstrap(); err != nil {
			return errors.New("Bootstrap: " + err.Error())
		}
	} else {
		if this.options.Interactif {
			go this.Cli()
		}
	}

	go func() {
		if err := this.loop(); err != nil {
			this.running = false
			this.logger.Error("Main loop: " + err.Error())
		}
	}()

	return nil
}

func (this *Dht) loop() error {
	this.running = true

	defer this.server.Close()

	for this.running {
		conn, err := this.server.Accept()

		if err != nil {
			return errors.New("Error accepting:" + err.Error())
		}

		go this.newConnection(conn)
	}

	return nil
}

func (this *Dht) Stop() {
	if !this.running {
		return
	}

	this.running = false
}

func (this *Dht) newConnection(conn net.Conn) {
	socket := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	node := NewNodeSocket(this, conn.RemoteAddr().String(), "", socket)

	err := node.Attach()

	if err != nil {
		this.logger.Error("Error attach", err.Error())

		return
	}

}

func (this *Dht) Logger() *logging.Logger {
	return this.logger
}

func (this *Dht) CustomCmd(data interface{}) {
	bucket := this.routing.FindNode(this.hash)

	for _, node := range bucket {
		res := node.Custom(data)
		fmt.Println(res)
	}
}

func (this *Dht) Broadcast(data interface{}) {

}

func (this *Dht) Running() bool {
	return this.running
}

func (this *Dht) Wait() {
	for this.running {
		time.Sleep(time.Second)
	}
}

func (this *Dht) OnCustomCmd(packet Packet) {
	this.options.OnCustomCmd(packet)
}
