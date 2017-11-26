package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/champii/go-dht/dht"
	"github.com/urfave/cli"
)

func main() {
	parseArgs(func(options dht.DhtOptions, c *cli.Context) {
		if options.Cluster > 0 {
			cluster(options)
		} else {
			node := startOne(options)

			if c.Bool("s") {
				storeFromStdin(node)
			} else if len(c.String("S")) > 0 {
				storeAt(node, c.String("S"))
			} else if len(c.String("f")) > 0 {
				fetchFromHash(node, c.String("f"))
			} else if len(c.String("F")) > 0 {
				fetchAt(node, c.String("F"))
			} else {
				node.Wait()
			}

			listenExitSignals(node)
		}
	})
}

func storeFromStdin(node *dht.Dht) {
	res, err := ioutil.ReadAll(os.Stdin)

	if err != nil {
		node.Logger().Critical("Cannot store", err)

		return
	}

	hash, nb, err := node.Store(res)

	if err != nil {
		node.Logger().Critical("Cannot store", err)

		return
	}

	if nb == 0 {
		node.Logger().Critical("Cannot store, no nodes found")

		return
	}

	fmt.Println(hex.EncodeToString(hash))
}

func storeAt(node *dht.Dht, hashStr string) {
	res, err := ioutil.ReadAll(os.Stdin)

	if err != nil {
		node.Logger().Critical("Cannot store", err)

		return
	}

	hash := dht.NewHash([]byte(hashStr))

	hash, nb, err := node.StoreAt(hash, res)

	if err != nil {
		node.Logger().Critical("Cannot store", err)

		return
	}

	if nb == 0 {
		node.Logger().Critical("Cannot store, no nodes found")

		return
	}

	fmt.Println(hex.EncodeToString(hash))
}

func fetchFromHash(node *dht.Dht, hashStr string) {
	hash, err := hex.DecodeString(hashStr)

	if err != nil {
		node.Logger().Critical("Cannot fetch", err)

		return
	}

	res, err := node.Fetch(hash)

	if err != nil {
		node.Logger().Critical("Cannot fetch", err)

		return
	}

	fmt.Print(string(res.([]byte)))
}

func fetchAt(node *dht.Dht, hashStr string) {
	hash := dht.NewHash([]byte(hashStr))

	res, err := node.Fetch(hash)

	if err != nil {
		node.Logger().Critical("Cannot fetch", err)

		return
	}

	fmt.Print(string(res.([]byte)))
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
