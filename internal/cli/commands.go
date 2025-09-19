package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/RasSved/D7024E_lab/internal/kademlia"
)

type NodeType interface {
	Put(data []byte) (string, error)
	Get(hash string) ([]byte, *kademlia.Contact, error)
}

func RunCommands(node NodeType) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				fmt.Println("input error:", err)
			}
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		cmd := parts[0]

		switch cmd {
		case "put":
			if len(parts) < 2 {
				fmt.Println("usage: put <string>")
				continue
			}
			hash, err := node.Put([]byte(parts[1]))
			if err != nil {
				fmt.Println("Error storing:", err)
			} else {
				fmt.Println("Stored with hash:", hash)
			}

		case "get":
			if len(parts) < 2 {
				fmt.Println("usage: get <hash>")
				continue
			}
			data, from, err := node.Get(parts[1])
			if err != nil {
				fmt.Println("Error retrieving:", err)
			} else if from == nil {
				fmt.Printf("Object found locally: %s\n", string(data))
			} else {
				fmt.Printf("Got from %s: %s\n", from.Address, string(data))
			}

		case "exit":
			fmt.Println("Shutting down…")
			return

		default:
			fmt.Println("Unknown command:", cmd)
		}
	}
}
