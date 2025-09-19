package main

import (
	"flag"
	"fmt"

	"github.com/RasSved/D7024E_lab/internal/cli"
	"github.com/RasSved/D7024E_lab/internal/kademlia"
)

// Initialization finns i func Listen(ip string, port int) *Network { rad 63 i network.go
func main() {
	port := flag.String("port", "8000", "listen port")
	bootstrap := flag.String("bootstrap", "", "bootstrap host:port")
	flag.Parse()

	me := kademlia.NewContact(kademlia.NewRandomKademliaID(), "127.0.0.1:"+*port)
	node := kademlia.New(me)

	if *bootstrap != "" {
		b := kademlia.NewContact(kademlia.NewRandomKademliaID(), *bootstrap)
		node.Join(&b)
		fmt.Println("Joined via", *bootstrap)
	} else {
		fmt.Println("Started at", me.Address)
	}

	cli.RunCommands(node)
}
