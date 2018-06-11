package dht

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/smallnest/rpcx/log"
	"github.com/smallnest/rpcx/server"
	kcp "github.com/xtaci/kcp-go"
	"golang.org/x/crypto/pbkdf2"

	logging "github.com/op/go-logging"
)

type Dht struct {
	sync.RWMutex
	routing      *Routing
	options      DhtOptions
	hash         []byte
	running      bool
	store        map[string][]byte
	logger       *logging.Logger
	server       *server.Server
	gotBroadcast [][]byte
	middlewares  []IMiddleware
}

type DhtOptions struct {
	NoRepublishOnExit bool
	ListenAddr        string
	BootstrapAddr     string
	Verbose           int
	Cluster           int
	Stats             bool
	Interactif        bool
	MaxStorageSize    int
	MaxItemSize       int
}

func New(options DhtOptions) *Dht {
	if options.MaxStorageSize == 0 {
		options.MaxStorageSize = 500000000 // ~500Mo
	}

	if options.MaxItemSize == 0 {
		options.MaxItemSize = 5000000 // ~5Mo
	}

	res := &Dht{
		routing: NewRouting(),
		options: options,
		running: false,
		store:   make(map[string][]byte),
		logger:  logging.MustGetLogger("dht"),
	}

	initLogger(res)

	res.routing.dht = res

	res.logger.Debug("DHT version 0.0.1")

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

var (
	pass  = pbkdf2.Key([]byte(cryptKey), []byte(cryptSalt), 4096, 32, sha1.New)
	bc, _ = kcp.NewAESBlockCrypt(pass)
)

const cryptKey = "rpcx-key"
const cryptSalt = "rpcx-salt"

func (this *Dht) Start() error {
	if this.running {
		return errors.New("Already started")
	}

	this.hash = NewRandomHash()

	r := rand.Intn(60) - 60
	timer := time.NewTicker(time.Minute*10 + (time.Second * time.Duration(r)))

	go func() {
		for range timer.C {
			this.republish()
		}
	}()

	this.logger.Info("Own hash", hex.EncodeToString(this.hash))

	go func() {
		log.SetDummyLogger()
		this.server = server.NewServer(server.WithBlockCrypt(bc))
		service := &Service{
			Dht: this,
		}

		err := this.server.RegisterName("Service", service, "")

		if err != nil {
			fmt.Println("Error registering: " + err.Error())

			return
		}

		err = this.server.Serve("kcp", this.options.ListenAddr)

		if err != nil {
			fmt.Println("Error listening: " + err.Error())

			return
		}

	}()

	this.logger.Info("Listening on " + this.options.ListenAddr)
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

func (this *Dht) Logger() *logging.Logger {
	return this.logger
}

// func (this *Dht) CustomCmd(data interface{}) {
// 	bucket := this.routing.FindNode(this.hash)

// 	for _, contact := range bucket {
// 		addr, _ := net.ResolveUDPAddr("udp", contact.Addr)

// 		node := NewNodeContact(this, addr, contact)
// 		<-node.Custom(data)
// 	}
// }

// func (this *Dht) hasBroadcast(hash []byte) bool {
// 	for _, h := range this.gotBroadcast {
// 		if compare(h, hash) == 0 {
// 			return true
// 		}
// 	}

// 	return false
// }

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

// func (this *Dht) Broadcast(data interface{}) {
// 	bucket := this.routing.FindNode(this.hash)

// 	var packet Packet
// 	switch data.(type) {
// 	case Packet_Broadcast:
// 		packet = data.(Packet)
// 	case Packet:
// 		packet = data.(Packet)
// 	default:
// 		// Fixme
// 		h, err := msgpack.Marshal(data)

// 		if err != nil {
// 			this.logger.Error("Cannot broadcast")

// 			return
// 		}

// 		packet = NewPacket(this, Command_BROADCAST, []byte{}, &Packet_Broadcast{&Broadcast{h}})
// 	}

// 	for _, contact := range bucket {
// 		addr, _ := net.ResolveUDPAddr("udp", contact.Addr)

// 		node := NewNodeContact(this, addr, contact)
// 		node.Broadcast(packet)
// 	}
// }

func (this *Dht) Running() bool {
	return this.running
}

func (this *Dht) Wait() {
	for this.running {
		time.Sleep(time.Second)
	}
}

// func (this *Dht) onCustomCmd(packet Packet) interface{} {
// 	for _, m := range this.middlewares {
// 		if res := m.OnCustomCmd(packet); res != nil {
// 			return res
// 		}
// 	}

// 	return nil
// }

// func (this *Dht) onBroadcast(packet Packet) interface{} {
// 	for _, m := range this.middlewares {
// 		if res := m.OnBroadcast(packet); res != nil {
// 			return res
// 		}
// 	}

// 	return nil
// }

func reverse(middle []IMiddleware) []IMiddleware {
	if len(middle) <= 1 {
		return middle
	}

	newmiddle := make([]IMiddleware, len(middle))

	for i, j := 0, len(middle)-1; i < j; i, j = i+1, j-1 {
		newmiddle[i], newmiddle[j] = middle[j], middle[i]
	}

	return newmiddle
}

func (this *Dht) onStore(packet *StoreRequest) bool {
	for _, m := range reverse(this.middlewares) {
		if res := m.OnStore(packet); res != true {
			return res
		}
	}

	return true
}

func (this *Dht) beforeSendStore(packet *StoreRequest) *StoreRequest {
	for _, m := range this.middlewares {
		if res := m.BeforeSendStore(packet); res != nil {
			packet = res
		}
	}

	return packet
}

func (this *Dht) GetConnectedNumber() int {
	return this.routing.Size()
}

func (this *Dht) StoredKeys() int {
	return len(this.store)
}

func (this *Dht) Storage() map[string][]byte {
	return this.store
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

func Compare(a []byte, b []byte) int {
	return compare(a, b)
}
