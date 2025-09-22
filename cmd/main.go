package main

import (
	"log"

	"github.com/RasSved/D7024E_lab/internal/cli"
)

// Initialization finns i func Listen(ip string, port int) *Network { rad 63 i network.go
//func main() {
//	port := flag.String("port", "8000", "listen port")
//	bootstrap := flag.String("bootstrap", "", "bootstrap host:port")
//	flag.Parse()
//
//	me := kademlia.NewContact(kademlia.NewRandomKademliaID(), "127.0.0.1:"+*port)
//	node := kademlia.New(me)
//
//	if *bootstrap != "" {
//		b := kademlia.NewContact(kademlia.NewRandomKademliaID(), *bootstrap)
//		node.Join(&b)
//		fmt.Println("Joined via", *bootstrap)
//	} else {
//		fmt.Println("Started at", me.Address)
//	}
//
//	cli.RunCommands(node)
//}

//func sha1Hex(b []byte) string {
//	h := sha1.Sum(b)
//	return hex.EncodeToString(h[:])
//}
//
//func mustContact(addr string, id *kademlia.KademliaID) kademlia.Contact {
//	if id == nil {
//		id = kademlia.NewRandomKademliaID()
//	}
//	return kademlia.NewContact(id, addr)
//}
//
//func main() {
//	// 1) Start two UDP nodes
//	n1 := kademlia.Listen("127.0.0.1", 4001)
//	n2 := kademlia.Listen("127.0.0.1", 4002)
//	defer n1.Close()
//	defer n2.Close()
//
//	// 2) Add each other to routing tables so they know how to talk
//	c1 := mustContact(n1.Address(), n1.ID())
//	c2 := mustContact(n2.Address(), n2.ID())
//	n1.RoutingTable.AddContact(c2)
//	n2.RoutingTable.AddContact(c1)
//
//	// 3) PING/PONG sanity check
//	if err := n1.SendPingMessage(&c2); err != nil {
//		log.Fatalf("PING failed: %v", err)
//	}
//	time.Sleep(200 * time.Millisecond) // tiny wait for log readability
//
//	// 4) FIND_NODE sync (n1 asks n2 about c1)
//	nodes, err := n1.SendFindContactMessage(&c2, c1.ID)
//	if err != nil {
//		log.Fatalf("FIND_NODE failed: %v", err)
//	}
//	fmt.Printf("FIND_NODE got %d nodes back\n", len(nodes))
//
//	// 5) STORE from n1, then FIND_VALUE from n2
//	data := []byte("hello-world")
//	key := sha1Hex(data) // must match your network.hashData
//	if err := n1.SendStoreMessage(data); err != nil {
//		log.Fatalf("STORE failed: %v", err)
//	}
//	time.Sleep(200 * time.Millisecond)
//
//	got, err := n2.SendFindDataMessage(key)
//	if err != nil {
//		log.Fatalf("FIND_VALUE failed: %v", err)
//	}
//	fmt.Printf("FIND_VALUE OK: %q\n", string(got))
//}

func main() {
	root := cli.NewRootCmd() // no node passed in; Cobra will build it
	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}
