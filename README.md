# go-dht
DHT implementation in GO

# Intro

Basic DHT implementation in GO, based on the Kademilia specifications.

# Usage

```
NAME:
  DHT - Experimental Distributed Hash Table

USAGE:
  go-dht [options]

VERSION:
  0.0.1

OPTIONS:
  -b value, --bootstrap value  Connect to bootstrap node ip:port
  -p value, --port value       Listening port (default: "0.0.0.0:3000")
  -i                           Interactif
  -s                           Stat mode
  -q, --quiet                  Quiet
  -n nodes, --network nodes    Spawn X new nodes network. If -b is not specified, a new network is created. (default: 0)
  -v level, --verbose level    Verbose level, 0 for CRITICAL and 5 for DEBUG (default: 4)
  -h, --help                   Print help
  -V, --version                Print version

```

# Basics

### Launch a network of 3 nodes (including a bootstrap node) with default options:

```bash
$> go build && ./go-dht -n 3
19:45:57.136 ▶ INFO 001 Listening on 0.0.0.0:3000
19:45:58.136 ▶ INFO 002 Listening on 0.0.0.0:3001
19:45:58.137 ▶ INFO 003 Ready...
19:45:58.236 ▶ INFO 004 Listening on 0.0.0.0:3002
19:45:58.240 ▶ INFO 005 Ready...

```

### Interactive console to connect to the network

```
$> go build && ./go-dht -b 0.0.0.0:3000 -p 0.0.0.0:6000 -i
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
		fmt.Println(err)

		return
	}

	hash, _ := client.Store("Some value")
	value, _ := client.Fetch(hash)

	fmt.Println(value) // Prints 'Some value'
}
```

# Todo

- keep old blocks in bucket (keep it sorted tho)
- Little sleep on republish and little range in timer
- Performances (better algo)
- Avoid value change rewrite
- BlackList for bad nodes (too many bad or incorrect answers)
- Custom commands
- Broadcast
- Cryptography ?
- Mirror Node (keeps all keys he finds)
- Proxy Node (for NAT Traversal)
- Debug Node that gets all infos from every nodes (Add a debug mode to do so)
- UDP instead of TCP ?
