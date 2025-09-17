package kademlia

import (
	"context"
	"sort"
	"time"
)

const (
	k          = 20
	a          = 3
	rpcTimeout = 2 * time.Second
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

func (kademlia *Kademlia) iterativeLookupContact(target *Contact) []Contact {
	if kademlia == nil || kademlia.routingTable == nil || target == nil || target.ID == nil {
		return nil
	}
	targetID := target.ID

	// 1. Find our k closest
	shortlist := kademlia.routingTable.FindClosestContacts(targetID, k)

	queried := make(map[string]bool, len(shortlist))

	for {
		// 2. Pick alpha closest unquired
		batch := make([]Contact, 0, a)
		for _, contact := range shortlist {
			if contact.ID == nil {
				continue
			}
			if !queried[contact.ID.String()] {
				batch = append(batch, contact)
				if len(batch) == a {
					break
				}
			}
		}
		if len(batch) == 0 {
			break
		}

		// 3. For each alpha picked send one RPC
		type res struct{ contacts []Contact }
		results := make(chan res, len(batch))
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)

		for _, to := range batch {
			to := to
			queried[to.ID.String()] = true
			go func() {
				cs, _ := kademlia.SendFindContactMessage(ctx, &to, targetID) // From network.go
				results <- res{contacts: cs}
			}()
		}

		// 5. Merge repelies
		progress := false
		for i := 0; i < len(batch); i++ {
			select {
			case r := <-results:
				if added := mergeKeepBestKNoSort(&shortlist, r.contacts, k, func(c *Contact) {
					c.CalcDistance(targetID)
				}); added {
					progress = true
				}
			case <-ctx.Done():
			}
		}
		cancel()

		// 5. Stop when no closer are found or all nodes are queried
		if progress {
			sort.Slice(shortlist, func(i, j int) bool {
				return shortlist[i].Less(&shortlist[j])
			})

			if len(shortlist) > k {
				shortlist = shortlist[:k]
			}
		}

		if !progress || kClosestAllQueried(shortlist, queried, k) {
			break
		}
	}
	return shortlist
}

func mergeKeepBestKNoSort(a *[]Contact, b []Contact, ksize int, distFn func(*Contact)) bool {
	if len(b) == 0 {
		return false
	}
	seen := make(map[string]bool, len(*a)+len(b))
	for _, c := range *a {
		if c.ID != nil {
			seen[c.ID.String()] = true
		}
	}
	added := false
	for _, c := range b {
		if c.ID == nil {
			continue
		}
		id := c.ID.String()
		if seen[id] {
			continue
		}
		seen[id] = true
		if distFn != nil {
			distFn(&c)
		}
		*a = append(*a, c)
		added = true
	}

	if len(*a) > ksize*2 {
		*a = (*a)[:ksize*2]
	}
	return added
}

func kClosestAllQueried(shortlist []Contact, queried map[string]bool, ksize int) bool {
	n := ksize
	if n > len(shortlist) {
		n = len(shortlist)
	}
	for i := 0; i < n; i++ {
		id := shortlist[i].ID
		if id != nil && !queried[id.String()] {
			return false
		}
	}
	return true
}
