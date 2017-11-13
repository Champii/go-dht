package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/champii/go-dht/dht"
)

func main() {
	// sizes := []int{3, 4, 5, 10, 20, 50, 100}
	sizes := []int{3}

	for _, size := range sizes {
		launchNetwork(size)

		client := launchNode(1000, dht.DhtOptions{
			ListenAddr:    "0.0.0.0:4000",
			BootstrapAddr: "0.0.0.0:3000",
			Verbose:       3,
			Stats:         false,
			Interactif:    false,
		})

		seed(size, client)

		// stopNetwork(net)
	}

	for {
		time.Sleep(time.Second)
	}
}

func seed(nb int, client *dht.Dht) {
	for i := 0; i < nb; i++ {
		client.Store(dht.NewHash([]byte(strconv.Itoa(nb))), nb, func(count int, err error) {

		})

	}

}

func launchNetwork(size int) []*dht.Dht {
	var net []*dht.Dht

	opts := dht.DhtOptions{
		ListenAddr:    "0.0.0.0:3000",
		BootstrapAddr: "",
		Verbose:       3,
		Stats:         false,
		Interactif:    false,
	}

	bNode := dht.New(opts)

	net = append(net, bNode)

	go bNode.Start()

	opts.BootstrapAddr = opts.ListenAddr

	for i := 1; i < size; i++ {
		node := launchNode(i, opts)

		net = append(net, node)

		time.Sleep(time.Millisecond * 400)
	}

	return net
}

func launchNode(i int, options dht.DhtOptions) *dht.Dht {
	addrPort := strings.Split(options.ListenAddr, ":")

	addr := addrPort[0]

	port, _ := strconv.Atoi(addrPort[1])

	options.ListenAddr = addr + ":" + strconv.Itoa(port+i)

	d := dht.New(options)

	go d.Start()

	return d
}

func stopNetwork(net []*dht.Dht) {
	for _, node := range net {
		node.Stop()
	}
}
