package dht

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

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
			if len(splited) != 3 {
				fmt.Println("Usage: s key value")
				prompt()
			}

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
			this.Fetch(NewHash([]byte(splited[1])), func(v interface{}, err error) {
				if err != nil {
					fmt.Println(err.Error())

					prompt()
					return
				}

				fmt.Println(splited[1], ":", v)

				prompt()

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
