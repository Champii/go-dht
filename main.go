package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/champii/dht-custom/dht"
	"github.com/urfave/cli"
)

func prepareArgs() *cli.App {
	cli.AppHelpTemplate = `NAME:
	{{.Name}} - {{.Usage}}

USAGE:
	{{if .VisibleFlags}}{{.HelpName}} [options] [files]{{end}}
	{{if len .Authors}}
AUTHOR:
	{{range .Authors}}{{ . }}{{end}}
	{{end}}{{if .Commands}}
VERSION:
	{{.Version}}

OPTIONS:
	{{range .VisibleFlags}}{{.}}
	{{end}}{{end}}{{if .Copyright }}

COPYRIGHT:
	{{.Copyright}}
	{{end}}{{if .Version}}
	{{end}}`

	cli.VersionFlag = cli.BoolFlag{
		Name:  "V, version",
		Usage: "Print version",
	}

	cli.HelpFlag = cli.BoolFlag{
		Name:  "h, help",
		Usage: "Print help",
	}

	app := cli.NewApp()

	app.Name = "DHT"
	app.Version = "0.0.1"
	app.Compiled = time.Now()

	app.Usage = "Experimental DHT"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "b, bootstrap",
			Usage: "Connect to bootstrap node ip:port",
		},
		cli.StringFlag{
			Name:  "p, port",
			Usage: "Listening port",
			Value: "0.0.0.0:3000",
		},
		cli.BoolFlag{
			Name:  "i",
			Usage: "Interactif",
		},
		cli.BoolFlag{
			Name:  "s",
			Usage: "Stat mode",
		},
		cli.BoolFlag{
			Name:  "q, quiet",
			Usage: "Quiet",
		},
		cli.IntFlag{
			Name:  "n, network",
			Value: 0,
			Usage: "Spawn X new `nodes` network. If -b is not specified, a new network is created.",
		},
		cli.IntFlag{
			Name:  "v, verbose",
			Value: 4,
			Usage: "Verbose `level`, 0 for CRITICAL and 5 for DEBUG",
		},
	}

	app.UsageText = "dht [options]"

	return app
}

func manageArgs() {
	app := prepareArgs()

	app.Action = func(c *cli.Context) error {
		options := dht.DhtOptions{
			ListenAddr:    c.String("p"),
			BootstrapAddr: c.String("b"),
			Verbose:       c.Int("v"),
			Stats:         c.Bool("s"),
			Interactif:    c.Bool("i"),
		}

		if c.Int("n") > 0 {
			cluster(c.Int("n"), options)

			return nil
		}

		dht := dht.New(options)

		dht.Start()

		return nil
	}

	app.Run(os.Args)
}

func main() {
	manageArgs()

}

func cluster(count int, options dht.DhtOptions) {
	network := []*dht.Dht{}
	i := 0
	if len(options.BootstrapAddr) == 0 {
		dht := dht.New(options)

		go dht.Start()

		time.Sleep(time.Second)

		network = append(network, dht)

		options.BootstrapAddr = options.ListenAddr

		i++
	}

	for ; i < count; i++ {
		options2 := options

		addrPort := strings.Split(options.ListenAddr, ":")

		addr := addrPort[0]

		port, _ := strconv.Atoi(addrPort[1])

		options2.ListenAddr = addr + ":" + strconv.Itoa(port+i)

		dht := dht.New(options2)

		go dht.Start()

		time.Sleep(time.Second * 2)

		network = append(network, dht)
	}

	for {
		time.Sleep(time.Second)
	}
}
