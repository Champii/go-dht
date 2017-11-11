package dht

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	logging "github.com/op/go-logging"
)

type Dht struct {
	routing *Routing
	options DhtOptions
	hash    string
	running bool
	store   map[string]interface{}
	logger  *logging.Logger
}

type DhtOptions struct {
	ListenAddr    string
	BootstrapAddr string
	Verbose       int
	Stats         bool
	Interactif    bool
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

func (this *Dht) Store(hash string, value interface{}, cb func(count int, err error)) {
	bucket := this.routing.FindNode(hash)

	this.fetchNodes(hash, bucket, []*Node{}, []*Node{}, func(foundNodes []*Node, err error) {
		if err != nil {
			cb(0, err)

			return
		}

		answer := 0

		for _, node := range foundNodes {
			node.Store(hash, value, func(val Packet, err error) {
				answer++
				if err != nil {
					cb(0, err)

					return
				}

				if answer == len(foundNodes) {
					cb(answer, err)
				}
			})
		}
	})
}

func (this *Dht) Fetch(hash string, done func(interface{}, error)) {
	bucket := this.routing.FindNode(hash)
	this.fetchNodes(hash, bucket, []*Node{}, []*Node{}, func(bucket []*Node, err error) {
		if err != nil {
			done(nil, err)

			return
		}

		answers := 0

		for _, n := range bucket {
			n.Fetch(hash, func(v Packet, err error) {
				answers++

				if err != nil {
					done(nil, err)

					return
				}

				if v.Header.Command == COMMAND_FOUND {
					done(v.Data, nil)

					return
				}

				if answers == len(bucket) {
					done(nil, errors.New("Not found"))

					return
				}
			})
		}
	})
}

func (this *Dht) processFoundBucket(hash string, foundNodes []PacketContact, best []*Node, blacklist []*Node, done func([]*Node, error)) {
	if len(foundNodes) == 0 {
		this.performConnectToAll(hash, foundNodes, best, blacklist, done)

		return
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
		done(best, nil)

		return
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
		done(best, nil)

		return
	}

	this.performConnectToAll(hash, foundNodes, best, blacklist, done)
}

func (this *Dht) fetchNodes(hash string, bucket []*Node, best []*Node, blacklist []*Node, done func([]*Node, error)) {
	var foundNodes []PacketContact

	nbAnswers := 0

	if len(bucket) == 0 {
		done(best, nil)

		return
	}

	for _, node := range bucket {
		node.FetchNodes(hash, func(val Packet, err error) {
			nbAnswers++

			if err != nil {
				this.logger.Error(err.Error())
				done(nil, err)

				return
			}

			foundNodes = append(foundNodes, val.Data.([]PacketContact)...)

			if nbAnswers == len(bucket) {
				this.processFoundBucket(hash, foundNodes, best, blacklist, done)
			}
		})
	}
}

func (this *Dht) contains(bucket []*Node, node PacketContact) bool {
	for _, n := range bucket {
		if node.Hash == n.contact.Hash {
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

func (this *Dht) performConnectToAll(hash string, foundNodes []PacketContact, best []*Node, blacklist []*Node, done func([]*Node, error)) {
	this.connectToAll(foundNodes, func(connectedNodes []*Node, err error) {
		if err != nil {
			done([]*Node{}, err)

			return
		}

		blacklist := append(blacklist, connectedNodes...)

		smalest := len(connectedNodes)

		if smalest > BUCKET_SIZE {
			smalest = BUCKET_SIZE
		}

		best = append(best, connectedNodes...)[:smalest]

		this.fetchNodes(hash, connectedNodes, best, blacklist, done)
	})

}

func (this *Dht) connectToAll(foundNodes []PacketContact, done func([]*Node, error)) {
	newBucket := []*Node{}

	for _, foundNode := range foundNodes {
		n := this.routing.GetNode(foundNode.Hash)

		if n == nil {
			n = NewNode(this, foundNode.Addr, foundNode.Hash)
		}

		newBucket = append(newBucket, n)
	}

	this.connectBucketAsync(newBucket, func(connectedNodes []*Node, err error) {
		if err != nil {
			done([]*Node{}, err)

			return
		}

		done(connectedNodes, nil)
	})
}

func (this *Dht) connectBucketAsync(bucket []*Node, cb func([]*Node, error)) {
	var res []*Node

	answers := 0

	if len(bucket) == 0 {
		cb(res, nil)

		return
	}

	for _, node := range bucket {
		node.Connect(func(val Packet, err error) {
			answers++

			if err != nil {
				cb(nil, err)

				return
			}

			res = append(res, node)

			if answers == len(bucket) {
				cb(res, nil)
			}
		})
	}
}

func (this *Dht) bootstrap() {
	this.logger.Debug("Connecting to bootstrap node", this.options.BootstrapAddr)

	bootstrapNode := NewNode(this, this.options.BootstrapAddr, "")

	bootstrapNode.Connect(func(val Packet, err error) {
		if err != nil {
			this.logger.Critical(err)
			os.Exit(1)
		}

		bucket := this.routing.FindNode(this.hash)

		this.fetchNodes(this.hash, bucket, []*Node{}, []*Node{}, func(nodes []*Node, err error) {
			if err != nil {
				this.logger.Critical(err)

				os.Exit(1)
			}

			this.logger.Info("Ready...")

			if this.options.Interactif {
				go this.Cli()
			}

		})
	})
}

func (this *Dht) Start() {
	if this.running {
		return
	}

	this.hash = NewRandomHash()

	this.logger.Debug("Own hash", this.hash)

	l, err := net.Listen("tcp", this.options.ListenAddr)

	if err != nil {
		this.logger.Critical("Error listening:", err.Error())

		os.Exit(1)
	}

	this.logger.Info("Listening on " + this.options.ListenAddr)

	this.running = true

	if len(this.options.BootstrapAddr) > 0 {
		this.bootstrap()
	} else {
		if this.options.Interactif {
			go this.Cli()
		}
	}

	for this.running {
		conn, err := l.Accept()

		if err != nil {
			this.logger.Critical("Error accepting: ", err.Error())

			os.Exit(1)
		}

		go this.newConnection(conn)
	}

	defer l.Close()
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

	node.Connect(func(val Packet, err error) {
		if err != nil {
			this.logger.Error("ERROR CONECT", err.Error())
		}
	})
}

func (this *Dht) PrintRoutingTable() {
	this.routing.Print()
}

func (this *Dht) PrintLocalStore() {
	for k, v := range this.store {
		fmt.Println(k, v)
	}
}

func (this *Dht) Cli() {
	fmt.Println("Type 'h' to get help")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Split(bufio.ScanLines)

	var prompt func()
	prompt = func() {
		fmt.Print("$> ")

		if !scanner.Scan() {
			fmt.Println("ERROR SCAN")
		}

		ln := scanner.Text()

		splited := strings.Split(ln, " ")

		switch splited[0] {
		case "h":
			help()
			prompt()
		case "i":
			fmt.Println("INFO")
			prompt()
		case "r":
			this.PrintRoutingTable()
			prompt()
		case "s":
			this.Store(NewHash([]byte(splited[1])), splited[2], func(count int, err error) {
				if err != nil {
					fmt.Println(err.Error())

					prompt()
					return
				}

				fmt.Println("Stored")
				prompt()
			})
		case "f":
			printed := false

			this.Fetch(NewHash([]byte(splited[1])), func(v interface{}, err error) {
				if err != nil {
					fmt.Println(err.Error())

					prompt()
					return
				}

				if !printed {
					fmt.Println(splited[1], ":", v)
					printed = true
					prompt()
				}

			})
		case "l":
			this.PrintLocalStore()
			prompt()
		case "q":
			this.Stop()
			os.Exit(1)
		case "":
			prompt()
		default:
			fmt.Println("Unknown command", splited[0])
			prompt()
		}
	}

	prompt()
}

func help() {
	fmt.Println("Commands:")
	fmt.Println("  h            - This help")
	fmt.Println("  i            - Global info")
	fmt.Println("  r            - Print routing table")
	fmt.Println("  s key val    - Store")
	fmt.Println("  f key        - Fetch")
	fmt.Println("  l            - Print local store")
	fmt.Println("  q            - Quit")
}
