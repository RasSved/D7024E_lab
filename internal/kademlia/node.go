package kademlia

import "fmt"

// NewNode is the single source of truth for creating a running node.
// It opens UDP, builds the contact/RT, and optionally bootstraps.
func NewNode(listenIP string, port int, bootstrapAddr string) (*Kademlia, error) {
	netw := NewNetwork(listenIP, port)
	if netw == nil {
		return nil, fmt.Errorf("failed to start network")
	}

	me := NewContact(netw.ID(), netw.Address())
	rt := NewRoutingTable(me)

	netw.RoutingTable = rt

	node := &Kademlia{
		routingTable: rt,
		network:      netw,
		store:        make(map[string][]byte),
	}

	if bootstrapAddr != "" {
		b := NewContact(NewRandomKademliaID(), bootstrapAddr)
		node.Join(&b)                // add to RT + warmup
		_ = netw.SendPingMessage(&b) // optional nudge
	}
	return node, nil
}

// Close shuts down the network listener.
func (k *Kademlia) Close() error {
	if k.network != nil {
		return k.network.Close()
	}
	return nil
}
