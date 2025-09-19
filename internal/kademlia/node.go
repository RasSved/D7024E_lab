package kademlia

func New(me Contact) *Kademlia {
	rt := NewRoutingTable(me)
	net := NewNetwork(&me, rt)
	return &Kademlia{
		routingTable: rt,
		network:      net,
		store:        make(map[string][]byte),
	}
}
