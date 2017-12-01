# go-dht
Customisable experimental DHT implementation in GO

## JumpTo

- [Intro](#intro)
- [Basics](#basics)
- [Usage](#usage)
- [Api](#api)
- [Limits](#limits)
- [Todo](#todo)

## Intro

Basic DHT implementation in GO, based on the Kademlia specifications with some benefits.

The DHT is build for performances and customisability. By allowing the use of the
callback `OnStore` to decide if the content is to be saved, one can finely tune
the network to bend it to its needs.

Also, it allows to build a protocol on top of its own with `CustomCmd`, and to
`Broadcast` a packet to the whole network

## Basics

### Launch a network of 3 nodes (including a bootstrap node) with default options and a little verbose:

```bash
bash> ./go-dht -n 3 -v 4
19:45:57.136 ▶ INFO 001 Listening on :3000
19:45:58.136 ▶ INFO 002 Listening on :3001
19:45:58.137 ▶ INFO 003 Ready...
19:45:58.236 ▶ INFO 004 Listening on :3002
19:45:58.240 ▶ INFO 005 Ready...

```

### Use the CLI tool to store and fetch
```
bash> cat somefile | ./go-dht -c :3000 -l :6000 -s
92ba3721b20d13873730ce026db89920
bash> ./go-dht -c :3000 -l :6000 -f 92ba3721b20d13873730ce026db89920 > somefile
```

Default storage size: 500Mo. Max item size in storage: 5Mo

### But you can also name them
```
bash> cat somefile | ./go-dht -c :3000 -l :6000 -S filename
3eba3721b90d13873330de5c6db8a0b4
bash> ./go-dht -c :3000 -l :6000 -F filename > somefile
```

### Interactive console

```
bash> ./go-dht -c :3000 -l :6000 -i
Type 'h' to get help
$> h
Commands:
  h            - This help
  r            - Print routing table
  s val        - Store. Returns the hash and the number of OK answers
  f key        - Fetch
  l            - Print local store
  q            - Quit
$> s testValue
92ba3721b20d13873730ce026db89920 16
$> f 92ba3721b20d13873730ce026db89920
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
		ListenAddr:    ":6000",
		BootstrapAddr: ":3000",
	})

	// no error management for lisibility but you realy should.

	client.Start()
	
	hash, _, _ := client.Store("Some value")

  var value string
	client.Fetch(hash, &value)

	fmt.Println(value) // Prints 'Some value'
  
	client.Stop()
}
```

## Usage

```
NAME:
  DHT - Experimental Distributed Hash Table

USAGE:
  go-dht [options]

VERSION:
  0.2.0

OPTIONS:
  -c value, --connect value  Connect to bootstrap node ip:port
  -l value, --listen value   Listening address and port (default: ":3000")
  -i, --interactif           Interactif
  -s, --store                Store from Stdin
  -S key, --store-at key     Same as '-s' but store at given key
  -f hash, --fetch hash      Fetch hash and prints to Stdout
  -F key, --fetch-at key     Same as '-f' but fetch from given key
  -n nodes, --network nodes  Spawn X new nodes in a network. (default: 0)
  -v level, --verbose level  Verbose level, 0 for CRITICAL and 5 for DEBUG (default: 3)
  -h, --help                 Print help
  -V, --version              Print version
```

## API

```go
func New(DhtOptions) *Dht

func (*Dht) Start() error
func (*Dht) Stop()

func (*Dht) Store(interface{}) ([]byte, int, error)
func (*Dht) StoreAt([]byte, interface{}) ([]byte, int, error)

func (*Dht) Fetch([]byte, *interface{}) error

func (*Dht) CustomCmd(interface{})
func (*Dht) Broadcast(interface{})

func (*Dht) Logger() *logging.logger
func (*Dht) Running() bool
func (*Dht) Wait()
func (*Dht) GetConnectedNumber() int
func (*Dht) StoredKeys() int

```

## Limits

- The lib provides a `StoreAt()` API that must be used wisely. In fact, by allowing to 
store any content at a given key instead of hashing it breaks the
automatic repartition of the data accross the network, as one can choose to store some
files closed to each other or repetedly target a portion of the network. To be used 
in coordination with `OnStore` callback, which can decide if the content is to be stored.
- No NAT traversal, each node must be directly reachable. A Proxy mode is in dev
- Not tested with huge network and a lot of arrival/departure of nodes. Need tests for that



## Todo

- Announce store before actually send to avoid sending data a node can't store
- Store on disk 
- Storage spread when high demand (with timeout decay with distance over best storage)
- Give some keys to newly connected
- keep old nodes in bucket (keep it sorted tho) + spare list for excedent
- Performances (better algo)
- BlackList for bad nodes (too many bad or incorrect answers)
- Cryptography ?
- Mirror Node (keeps all keys he finds)
- Proxy Node (for NAT Traversal)
- Debug Node that gets all infos from every nodes (Add a debug mode to do so)
- key expire timeout ?
- Detect and prevent Byzantine attack failure
- Manual or reproducible serialization (avoid native `gob` feature)