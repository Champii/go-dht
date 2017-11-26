package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/champii/go-dht/dht"
)

func main() {
	sizes := []int{5, 10, 20, 50, 75}

	for _, size := range sizes {
		net := cluster(size, dht.DhtOptions{
			ListenAddr:        "0.0.0.0:3000",
			Verbose:           3,
			Stats:             false,
			Interactif:        false,
			NoRepublishOnExit: true,
		})

		seed(size*10, net[0])

		checkSeeds(size*10, net)

		stopNetwork(net)
		time.Sleep(time.Millisecond * 200)
	}
}

func seed(nb int, client *dht.Dht) {
	fmt.Print("Seeding: ")
	defer timeTrack(time.Now())

	for i := 0; i < nb; i++ {
		_, n, err := client.StoreAt(dht.NewHash([]byte(strconv.Itoa(i))), i)

		if err != nil || n == 0 {
			fmt.Println("Error seeding node", i, "on", nb, ":", n, err)

			os.Exit(0)
		}
	}
}

func checkSeeds(nb int, network []*dht.Dht) {
	fmt.Println("Checking...")
	defer timeTrack(time.Now())

	avgStorage := 0
	for nodeNb, node := range network {
		avgStorage += node.StoredKeys()
		for i := 0; i < nb; i++ {
			res, err := node.Fetch(dht.NewHash([]byte(strconv.Itoa(i))))

			if err != nil || res != i {
				fmt.Println(nodeNb, "Error getting value", i, "on", nb, ":", res, err)

				os.Exit(0)
			}
		}
	}

	fmt.Println("Average keys by node:", avgStorage/len(network))
}

func cluster(count int, options dht.DhtOptions) []*dht.Dht {
	fmt.Print("Start cluster of size ", count, ": ")
	defer timeTrack(time.Now())

	network := []*dht.Dht{}
	i := 0

	if len(options.BootstrapAddr) == 0 {
		client := startOne(options)

		network = append(network, client)

		options.BootstrapAddr = options.ListenAddr

		i++
	}

	for ; i < count; i++ {
		options2 := options

		addrPort := strings.Split(options.ListenAddr, ":")

		addr := addrPort[0]

		port, _ := strconv.Atoi(addrPort[1])

		options2.ListenAddr = addr + ":" + strconv.Itoa(port+i)

		client := startOne(options2)

		network = append(network, client)
	}

	return network
}

func startOne(options dht.DhtOptions) *dht.Dht {
	client := dht.New(options)

	if err := client.Start(); err != nil {
		client.Logger().Critical(err)
	}

	return client
}

func stopNetwork(net []*dht.Dht) {
	fmt.Print("Stoping network: ")
	defer timeTrack(time.Now())

	for _, node := range net {
		node.Stop()
	}
}

func timeTrack(start time.Time) {
	elapsed := time.Since(start)

	fmt.Println(elapsed)
	fmt.Println("")
}
