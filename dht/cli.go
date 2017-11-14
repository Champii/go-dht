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

	for this.running {
		fmt.Print("$> ")

		if !scanner.Scan() {
			fmt.Println("ERROR SCAN")
		}

		ln := scanner.Text()

		splited := strings.Split(ln, " ")

		switch splited[0] {
		case "h":
			help()
		case "i":
			fmt.Println("INFO")
		case "r":
			this.PrintRoutingTable()
		case "s":
			if len(splited) != 2 {
				fmt.Println("Usage: s value")
				continue
			}

			hash, err := this.Store(splited[1])
			if err != nil {
				fmt.Println(err.Error())

				continue
			}

			fmt.Println(hash)
		case "f":
			if len(splited) != 2 || len(splited[1]) != 64 {
				fmt.Println("Usage: f key")
				continue
			}

			val, err := this.Fetch(splited[1])
			if err != nil {
				fmt.Println(err.Error())

				continue
			}

			fmt.Println(val)

		case "l":
			this.PrintLocalStore()
		case "q":
			this.Stop()
			os.Exit(1)
		case "":
		default:
			fmt.Println("Unknown command", splited[0])
		}
	}
}

func help() {
	fmt.Println("Commands:")
	fmt.Println("  h            - This help")
	fmt.Println("  i            - Global info")
	fmt.Println("  r            - Print routing table")
	fmt.Println("  s val        - Store. Returns the hash of the stored item")
	fmt.Println("  f key        - Fetch")
	fmt.Println("  l            - Print local store")
	fmt.Println("  q            - Quit")
}
