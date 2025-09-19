package kademlia

// Join: populate RT by looking up your own ID
func (kademlia *Kademlia) Join(bootstrap *Contact) {
	if bootstrap != nil {
		kademlia.routingTable.AddContact(*bootstrap)
	}
	me := &Contact{ID: kademlia.routingTable.me.ID, Address: kademlia.routingTable.me.Address}
	_ = kademlia.iterativeLookupContact(me) // uses FIND_NODE over the network
}

// PUT: hash -> find k closest -> send STORE to them
//func (k *Kademlia) Put(data []byte) (string, error) {
//	sum := sha1.Sum(data)
//	key := hex.EncodeToString(sum[:])
//	keyID := NewKademliaID(key)
//
//	target := &Contact{ID: keyID}
//	closest := k.iterativeLookupContact(target) // client-side iterative FIND_NODE
//
//	// send STORE to each of the k nodes
//	for _, c := range closest {
//		//if err := k.network.SendStoreMessage(&c, key, data); err != nil { // Not yet implemented in network.go
//		// best-effort; you can count acks if you want
//		//}
//	}
//	return key, nil
//}
//
//// GET: iterative find-value: query peers; if any returns value, stop; else converge like find-node
//func (k *Kademlia) Get(key string) ([]byte, *Contact, error) {
//	//keyID := NewKademliaID(key)
//	// Start with k closest we currently know
//	// shortlist := k.iterativeFindValue(keyID)
//}
