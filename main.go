package main

import (
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/champii/go-dht/dht"
)

func main() {
	parseArgs(func(options dht.DhtOptions) {
		if options.Cluster > 0 {
			cluster(options)
		} else {
			node := startOne(options)

			node.Wait()

			listenExitSignals(node)
		}
	})
}

func listenExitSignals(client *dht.Dht) {
	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs

		exitProperly(client)

		os.Exit(0)
	}()
}

func exitProperly(client *dht.Dht) {
	client.Stop()
}

func cluster(options dht.DhtOptions) {
	network := []*dht.Dht{}
	i := 0

	if len(options.BootstrapAddr) == 0 {
		client := startOne(options)

		network = append(network, client)

		options.BootstrapAddr = options.ListenAddr

		i++
	}

	for ; i < options.Cluster; i++ {
		options2 := options

		addrPort := strings.Split(options.ListenAddr, ":")

		addr := addrPort[0]

		port, _ := strconv.Atoi(addrPort[1])

		options2.ListenAddr = addr + ":" + strconv.Itoa(port+i)

		client := startOne(options2)

		network = append(network, client)
	}

	for {
		time.Sleep(time.Second)
	}
}

func startOne(options dht.DhtOptions) *dht.Dht {
	client := dht.New(options)

	if err := client.Start(); err != nil {
		client.Logger().Critical(err)
	}

	return client
}
