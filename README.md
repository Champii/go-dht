# go-dht
DHT implementation in GO

## Intro

Basic DHT implementation in GO, based on the Kademlia specifications.

## Usage

```
NAME:
  DHT - Experimental Distributed Hash Table

USAGE:
  go-dht [options]

VERSION:
  0.1.1

OPTIONS:
  -c value, --connect value    Connect to bootstrap node ip:port
  -l value, --listen value     Listening addr:port (default: "0.0.0.0:3000")
  -i                           Interactif
  -s                           Stat mode
  -v level, --verbose level    Verbose level, 0 for CRITICAL and 5 for DEBUG (default: 3)
  -n nodes, --network nodes    Spawn X new nodes in a network. (default: 0)
  -h, --help                   Print help
  -V, --version                Print version
```

## Basics

### Launch a network of 3 nodes (including a bootstrap node) with default options and a little verbose:

```bash
$> go build && ./go-dht -n 3 -v 4
19:45:57.136 ▶ INFO 001 Listening on 0.0.0.0:3000
19:45:58.136 ▶ INFO 002 Listening on 0.0.0.0:3001
19:45:58.137 ▶ INFO 003 Ready...
19:45:58.236 ▶ INFO 004 Listening on 0.0.0.0:3002
19:45:58.240 ▶ INFO 005 Ready...

```

### Interactive console to connect to the network

```
$> go build && ./go-dht -c 0.0.0.0:3000 -l 0.0.0.0:6000 -i
19:43:06.711 ▶ INFO 001 Listening on 0.0.0.0:6000
19:43:06.720 ▶ INFO 002 Ready...
Type 'h' to get help
$> h
Commands:
  h            - This help
  i            - Global info
  r            - Print routing table
  s val        - Store. Returns the hash of the stored item
  f key        - Fetch
  l            - Print local store
  q            - Quit
$> s testValue
92ba3721b20d13873730ce026db89920b47379dc39797ce74b925a3017c2048f
$> f 92ba3721b20d13873730ce026db89920b47379dc39797ce74b925a3017c2048f
testValue
$>
```

### Create your own client to connect to the network

```go
package main

import (
	"fmt"

	"github.com/champii/go-dht/dht"
)
func main() {

	client := dht.New(dht.DhtOptions{
		ListenAddr:    "0.0.0.0:6000",
		BootstrapAddr: "0.0.0.0:3000",
	})

	if err := client.Start(); err != nil {
		return
	}

	hash, stored, err := client.Store("Some value")

	if err != nil || stored == 0 {
		return
	}

	value, err := client.Fetch(hash)

	if err != nil {
		return
	}

	fmt.Println(value) // Prints 'Some value'
}
```

## API

```go
func New(DhtOptions) *Dht

func (*Dht) Start() error
func (*Dht) Stop()

func (*Dht) Store(interface{}) ([]byte, int, error)
func (*Dht) StoreAt([]byte, interface{}) ([]byte, int, error)
func (*Dht) Fetch([]byte) ([]byte, int, error)

func (*Dht) CustomCmd(interface{})
func (*Dht) Broadcast(interface{})

func (*Dht) Logger() *logging.logger
func (*Dht) Running() bool
func (*Dht) Wait()
func (*Dht) GetConnectedNumber() int
func (*Dht) StoredKeys() int

```

## Todo

- Refuse connection from nodes when the hash exists already in routing
- Storage spread when high demand (with timeout decay with distance over best storage)
- Give some keys to newly connected
- keep old nodes in bucket (keep it sorted tho) + spare list for excedent
- Performances (better algo)
- BlackList for bad nodes (too many bad or incorrect answers)
- Cryptography ?
- Mirror Node (keeps all keys he finds)
- Proxy Node (for NAT Traversal)
- Debug Node that gets all infos from every nodes (Add a debug mode to do so)
- UDP instead of TCP ?
- key expire timeout ?
