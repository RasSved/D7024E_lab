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
	network      *Network
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

	// 1) Start with our k closest
	shortlist := kademlia.routingTable.FindClosestContacts(targetID, k)
	for i := range shortlist {
		shortlist[i].CalcDistance(targetID)
	}
	sort.Slice(shortlist, func(i, j int) bool { return shortlist[i].Less(&shortlist[j]) })

	queried := make(map[string]bool, len(shortlist))

	for {
		// 2) Pick α closest unqueried
		batch := make([]Contact, 0, a)
		for _, c := range shortlist {
			if c.ID == nil {
				continue
			}
			if c.ID.String() == kademlia.routingTable.me.ID.String() {
				continue
			} // skip self
			if !queried[c.ID.String()] {
				batch = append(batch, c)
				if len(batch) == a {
					break
				}
			}
		}
		if len(batch) == 0 || kClosestAllQueried(shortlist, queried, k) {
			return shortlist
		}

		// 3) Fire all α queries in parallel
		type res struct {
			from  Contact
			nodes []Contact
		}
		results := make(chan res, len(batch))
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		for _, peer := range batch {
			p := peer // capture
			ch, err := kademlia.network.SendFindContactMessage(&p, targetID)
			if err != nil {
				// mark as queried anyway to avoid retry-looping the bad peer
				queried[p.ID.String()] = true
				continue
			}
			go func() {
				select {
				case nodes, ok := <-ch:
					if ok {
						results <- res{from: p, nodes: nodes}
					} else {
						results <- res{from: p, nodes: nil}
					}
				case <-ctx.Done():
					results <- res{from: p, nodes: nil}
				}
			}()
		}

		// 4) Collect and merge
		progress := false
		for i := 0; i < len(batch); i++ {
			select {
			case r := <-results:
				if r.from.ID != nil {
					queried[r.from.ID.String()] = true
				}
				if len(r.nodes) > 0 {
					// compute distance and merge
					for i := range r.nodes {
						r.nodes[i].CalcDistance(targetID)
					}
					if mergeKeepBestKNoSort(&shortlist, r.nodes, k, func(c *Contact) {
						c.CalcDistance(targetID)
					}) {
						progress = true
					}
				}
			case <-ctx.Done():
				// timeout -> no nodes merged for this response
			}
		}
		cancel()

		// 5) Sort and decide whether to continue
		if progress {
			sort.Slice(shortlist, func(i, j int) bool { return shortlist[i].Less(&shortlist[j]) })
			continue
		}
		// no closer nodes found -> done
		return shortlist
	}
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
