package kademlia

const (
	k = 20
)

type Kademlia struct {
	routingTable *RoutingTable
	store        map[string][]byte
}

// ---------------------------- Server Side Logic ----------------------------------------------------------------

// Note: I have not make any test for this so might be wonky but idk looks right

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact { // Server side logic for FIND_NODE
	contacts := kademlia.routingTable.FindClosestContacts(target.ID, k)
	return contacts
}

func (kademlia *Kademlia) LookupData(hash string) ([]byte, []Contact) { // Server side logic for FIND_VALUE
	if val, ok := kademlia.store[hash]; ok {
		return val, nil
	}

	KeyID := NewKademliaID(hash)
	if KeyID == nil {
		return nil, nil
	}

	closest := kademlia.routingTable.FindClosestContacts(KeyID, k)
	return nil, closest
}

func (kademlia *Kademlia) Store(key string, data []byte) bool { // Server side logic for STORE_VALUE
	if kademlia.store == nil {
		kademlia.store = make(map[string][]byte)
	}

	value := make([]byte, len(data))
	copy(value, data)
	kademlia.store[key] = value
	return true
}

// ----------------------------- Client Side Logic ------------------------------------------------------------
