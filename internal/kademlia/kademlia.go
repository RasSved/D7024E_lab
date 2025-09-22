package kademlia

import (
	"context"
	"sort"
	"sync"
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
	network      NetworkAPI
	storeMutex   sync.RWMutex
}

type NetworkAPI interface {
	// used by iterativeLookupContact
	SendFindContactMessageAsync(peer *Contact, target *KademliaID) (<-chan []Contact, error)

	// used by Join/Put/Get
	SendPingMessage(contact *Contact) error
	SendStoreMessage(data []byte) error
	SendFindDataMessage(hash string) ([]byte, error)

	// misc used by Kademlia
	Address() string
	Close() error
}

// ---------------------------- Server Side Logic ----------------------------------------------------------------

// Note: I have not make any test for this so might be wonky but idk looks right

func (kademlia *Kademlia) LookupContact(target *Contact) []Contact { // Server side logic for FIND_NODE
	if kademlia == nil || kademlia.routingTable == nil || target == nil || target.ID == nil {
		return nil
	}
	contacts := kademlia.routingTable.FindClosestContacts(target.ID, k)
	return contacts
}

func (kademlia *Kademlia) LookupData(hash string) ([]byte, []Contact) { // Server side logic for FIND_VALUE
	// safe read under lock
	kademlia.storeMutex.RLock()
	if val, ok := kademlia.store[hash]; ok {
		v := make([]byte, len(val))
		copy(v, val)
		kademlia.storeMutex.RUnlock()
		return v, nil
	}
	kademlia.storeMutex.RUnlock()

	keyID := NewKademliaID(hash)
	if keyID == nil {
		return nil, nil
	}
	closest := kademlia.routingTable.FindClosestContacts(keyID, k)
	return nil, closest
}

func (kademlia *Kademlia) Store(key string, data []byte) bool { // Server side logic for STORE_VALUE
	if data == nil {
		return false
	}

	value := make([]byte, len(data))
	copy(value, data)
	kademlia.storeMutex.Lock()

	if kademlia.store == nil {
		kademlia.store = make(map[string][]byte)
	}

	kademlia.store[key] = value
	kademlia.storeMutex.Unlock()
	return true
}

// ----------------------------- Client Side Logic ------------------------------------------------------------

func (kademlia *Kademlia) iterativeLookupContact(target *Contact) []Contact {
	if kademlia == nil || kademlia.routingTable == nil || kademlia.network == nil || target == nil || target.ID == nil {
		return nil
	}
	targetID := target.ID

	// 1) start with our k closest
	shortlist := kademlia.routingTable.FindClosestContacts(targetID, k)
	for i := range shortlist {
		shortlist[i].CalcDistance(targetID)
	}
	sort.Slice(shortlist, func(i, j int) bool { return shortlist[i].Less(&shortlist[j]) })

	queried := make(map[string]bool, len(shortlist))

	for {
		// 2) pick α closest unqueried
		batch := make([]Contact, 0, a)
		for _, c := range shortlist {
			if c.ID == nil || c.ID.String() == kademlia.routingTable.me.ID.String() {
				continue
			}
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

		type res struct {
			from  Contact
			nodes []Contact
		}
		results := make(chan res, len(batch))
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)

		// 3) fire only for peers we successfully created a channel for
		started := 0
		for _, peer := range batch {
			p := peer
			ch, err := kademlia.network.SendFindContactMessageAsync(&p, targetID)
			if err != nil {
				queried[p.ID.String()] = true
				continue
			}
			started++
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

		progress := false
		for i := 0; i < started; i++ {
			select {
			case r := <-results:
				if r.from.ID != nil {
					queried[r.from.ID.String()] = true
				}
				if len(r.nodes) > 0 {
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
				// overall timeout; remaining iterations will hit Done immediately
			}
		}
		cancel()

		// sort and trim AFTER merging
		sort.Slice(shortlist, func(i, j int) bool { return shortlist[i].Less(&shortlist[j]) })
		if len(shortlist) > k {
			shortlist = shortlist[:k]
		}

		if !progress {
			return shortlist
		}
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

// Expose routing table
func (k *Kademlia) RoutingTable() *RoutingTable {
	return k.routingTable
}

// Expose network
func (k *Kademlia) Network() NetworkAPI {
	return k.network
}

func (k *Kademlia) Address() string {
	if k.network != nil {
		return k.network.Address()
	}
	return ""
}
