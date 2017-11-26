package main

import (
	"os"
	"time"

	"github.com/champii/go-dht/dht"
	"github.com/urfave/cli"
)

func prepareArgs() *cli.App {
	cli.AppHelpTemplate = `NAME:
	{{.Name}} - {{.Usage}}

USAGE:
	{{if .VisibleFlags}}{{.HelpName}} [options]{{end}}
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
	app.Version = "0.2.0"
	app.Compiled = time.Now()

	app.Usage = "Experimental Distributed Hash Table"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "c, connect",
			Usage: "Connect to bootstrap node ip:port",
		},
		cli.StringFlag{
			Name:  "l, listen",
			Usage: "Listening address and port",
			Value: ":3000",
		},
		cli.BoolFlag{
			Name:  "i, interactif",
			Usage: "Interactif",
		},
		cli.BoolFlag{
			Name:  "s, store",
			Usage: "Store from Stdin",
		},
		cli.StringFlag{
			Name:  "S, store-at",
			Usage: "Same as '-s' but store at given `key`",
		},
		cli.StringFlag{
			Name:  "f, fetch",
			Usage: "Fetch `hash` and prints to Stdout",
		},
		cli.StringFlag{
			Name:  "F, fetch-at",
			Usage: "Same as '-f' but fetch from given `key`",
		},
		cli.IntFlag{
			Name:  "n, network",
			Value: 0,
			Usage: "Spawn X new `nodes` in a network.",
		},
		cli.IntFlag{
			Name:  "v, verbose",
			Value: 3,
			Usage: "Verbose `level`, 0 for CRITICAL and 5 for DEBUG",
		},
	}

	app.UsageText = "dht [options]"

	return app
}

func parseArgs(done func(dht.DhtOptions, *cli.Context)) {
	app := prepareArgs()

	app.Action = func(c *cli.Context) error {
		options := dht.DhtOptions{
			ListenAddr:    c.String("l"),
			BootstrapAddr: c.String("c"),
			Verbose:       c.Int("v"),
			Stats:         c.Bool("s"),
			Interactif:    c.Bool("i"),
			Cluster:       c.Int("n"),
			// OnStore:       func(dht.Packet) interface{} {},
		}

		if options.Cluster > 0 {
			options.Stats = false
			options.Interactif = false
		}

		if options.Interactif {
			options.Stats = false
		}

		done(options, c)

		return nil
	}

	app.Run(os.Args)
}
