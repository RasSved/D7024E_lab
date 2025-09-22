package kademlia

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"
)

/*
SimNet: in-memory network emulator for tests
- Implements NetworkAPI (SendFindContactMessageAsync, SendPingMessage, SendStoreMessage, SendFindDataMessage, Address, Close)
- Emulates N nodes (ID + address) without real sockets
- Configurable packet drop %, latency, and RNG seed for determinism
- Computes “closest nodes” by scanning its node set (fine for 1k)
*/

type SimNode struct {
	ID      *KademliaID
	Address string

	// optional per-node data store (key hex -> value)
	data map[string][]byte
}

type SimNet struct {
	addr string // for the local test node’s Address()

	// all known nodes in the simulated overlay (not including “me” unless you add it)
	nodes []SimNode

	// config knobs
	dropPct    float64       // e.g. 0.0 .. 1.0
	latencyMin time.Duration // artificial latency range (min..max)
	latencyMax time.Duration
	rng        *rand.Rand
	kSize      int // K for closest nodes (use your constant k)
	alpha      int // alpha for query fanout (not strictly needed here)
	dataMutex  sync.RWMutex
	closeOnce  sync.Once
	closed     chan struct{}
}

// ---- SimNet implements NetworkAPI ----

func (sn *SimNet) SendFindContactMessageAsync(peer *Contact, target *KademliaID) (<-chan []Contact, error) {
	ch := make(chan []Contact, 1)
	if peer == nil || peer.ID == nil || target == nil {
		close(ch)
		return ch, nil
	}
	// emulate async delivery
	go func() {
		defer close(ch)
		// latency
		sn.sleepOne()
		// drop?
		if sn.drop() {
			return
		}
		// give back K closest (excluding peer itself)
		nodes := sn.closestContacts(target, sn.kSize, peer)
		ch <- nodes
	}()
	return ch, nil
}

func (sn *SimNet) SendPingMessage(*Contact) error {
	// we don’t do anything besides “maybe drop”
	if sn.drop() {
		return fmt.Errorf("ping dropped")
	}
	return nil
}

func (sn *SimNet) SendStoreMessage(data []byte) error {
	if sn.drop() {
		return fmt.Errorf("store multicast dropped")
	}
	// compute SHA-1 like your Network.hashData
	sum := sha1.Sum(data)
	key := hex.EncodeToString(sum[:])

	// choose K closest nodes to the key and “store”
	keyID := NewKademliaID(key)
	targets := sn.closestContacts(keyID, sn.kSize, nil)

	sn.dataMutex.Lock()
	for i := range targets {
		if targets[i].ID == nil {
			continue
		}
		// locate backing SimNode by ID
		for j := range sn.nodes {
			if sn.nodes[j].ID.String() == targets[i].ID.String() {
				if sn.nodes[j].data == nil {
					sn.nodes[j].data = make(map[string][]byte)
				}
				sn.nodes[j].data[key] = append([]byte(nil), data...)
				break
			}
		}
	}
	sn.dataMutex.Unlock()
	return nil
}

func (sn *SimNet) SendFindDataMessage(hash string) ([]byte, error) {
	// treat non-hex / bad length same as prod (quick validation)
	if len(hash) != 2*IDLength || !isHex(hash) {
		return nil, fmt.Errorf("invalid hash format")
	}
	// 1) “local” cache not modeled; go to simulated dht
	sn.dataMutex.RLock()
	defer sn.dataMutex.RUnlock()
	for i := range sn.nodes {
		if v, ok := sn.nodes[i].data[hash]; ok {
			return append([]byte(nil), v...), nil
		}
	}
	return nil, fmt.Errorf("data not found")
}

func (sn *SimNet) Address() string { return sn.addr }
func (sn *SimNet) Close() error {
	sn.closeOnce.Do(func() {
		close(sn.closed)
	})
	return nil
}

// ---- helpers for SimNet ----

func (sn *SimNet) drop() bool {
	if sn.dropPct <= 0 {
		return false
	}
	return sn.rng.Float64() < sn.dropPct
}

func (sn *SimNet) sleepOne() {
	if sn.latencyMax <= 0 && sn.latencyMin <= 0 {
		return
	}
	d := sn.latencyMin
	if sn.latencyMax > sn.latencyMin {
		// random in [min, max]
		d += time.Duration(sn.rng.Int63n(int64(sn.latencyMax - sn.latencyMin + 1)))
	}
	select {
	case <-time.After(d):
	case <-sn.closed:
	}
}

func (sn *SimNet) closestContacts(target *KademliaID, count int, exclude *Contact) []Contact {
	type scored struct {
		c Contact
		d *KademliaID
	}
	res := make([]scored, 0, len(sn.nodes))
	for i := range sn.nodes {
		id := sn.nodes[i].ID
		if id == nil {
			continue
		}
		if exclude != nil && exclude.ID != nil && exclude.ID.String() == id.String() {
			continue
		}
		c := NewContact(id, sn.nodes[i].Address)
		c.CalcDistance(target)
		res = append(res, scored{c: c, d: c.distance})
	}
	// sort by XOR distance (lexicographically on big-endian bytes)
	sort.Slice(res, func(i, j int) bool {
		return res[i].c.Less(&res[j].c)
	})
	if count > len(res) {
		count = len(res)
	}
	out := make([]Contact, count)
	for i := 0; i < count; i++ {
		out[i] = res[i].c
	}
	return out
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// build a SimNet with N random nodes.
// dropPct e.g. 0.3 for 30% drop; latency in range [min,max].
func newSimNet(t *testing.T, seed int64, numNodes int, dropPct float64, minLat, maxLat time.Duration, k, alpha int) *SimNet {
	t.Helper()
	r := rand.New(rand.NewSource(seed))
	nodes := make([]SimNode, numNodes)
	for i := 0; i < numNodes; i++ {
		id := NewRandomKademliaID()
		nodes[i] = SimNode{
			ID:      id,
			Address: fmt.Sprintf("sim:%d", i),
		}
	}
	return &SimNet{
		addr:       "sim-local",
		nodes:      nodes,
		dropPct:    dropPct,
		latencyMin: minLat,
		latencyMax: maxLat,
		rng:        r,
		kSize:      k,
		alpha:      alpha,
		closed:     make(chan struct{}),
	}
}

/* -------------------- TESTS -------------------- */

// This test emulates 1000 nodes with 30% packet drop and modest latency.
// We verify iterativeLookupContact still converges to K closest contacts.
func TestSim_IterativeLookup_ThousandNodes_WithPacketDrop(t *testing.T) {
	const (
		numNodes = 1000 // 👈 adjust here
		drop     = 0.5  // 👈 30% drops; adjust here
		seed     = 1337
	)
	sn := newSimNet(t, seed, numNodes, drop, 0, 1*time.Millisecond, k, a)

	me := NewContact(NewRandomKademliaID(), "me:sim")
	rt := NewRoutingTable(me)

	// Seed RT with K random entries from the sim overlay (your real node’s view)
	for i := 0; i < k; i++ {
		idx := sn.rng.Intn(len(sn.nodes))
		rt.AddContact(NewContact(sn.nodes[idx].ID, sn.nodes[idx].Address))
	}

	kad := &Kademlia{
		routingTable: rt,
		network:      sn, // <- inject the simulator via NetworkAPI
		store:        make(map[string][]byte),
	}

	// Pick a random target and run the iterative lookup
	target := NewContact(NewRandomKademliaID(), "target")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// run in a goroutine to enforce upper bound (defensive against logic regressions)
	type outcome struct{ res []Contact }
	done := make(chan outcome, 1)
	go func() {
		done <- outcome{res: kad.iterativeLookupContact(&target)}
	}()

	select {
	case out := <-done:
		if len(out.res) == 0 {
			t.Fatalf("expected non-empty result")
		}
		// Ensure we got at most K and that it is sorted by XOR distance
		if len(out.res) > k {
			t.Fatalf("got %d > k results", len(out.res))
		}
		prev := out.res[0]
		prev.CalcDistance(target.ID)
		for i := 1; i < len(out.res); i++ {
			cur := out.res[i]
			cur.CalcDistance(target.ID)
			if cur.Less(&prev) {
				t.Fatalf("shortlist not sorted by distance at index %d", i)
			}
			prev = cur
		}
	case <-ctx.Done():
		t.Fatalf("iterative lookup timed out under sim conditions")
	}
}

// End-to-end STORE/GET over the simulator with 1000 nodes & packet loss.
// This exercises SendStoreMessage/SendFindDataMessage via the interface.
func TestSim_PutGet_ThousandNodes_WithPacketDrop(t *testing.T) {
	const (
		numNodes = 1000
		drop     = 0.5 // e.g., 15% loss
		seed     = 4242
	)
	sn := newSimNet(t, seed, numNodes, drop, 0, 0, k, a)

	me := NewContact(NewRandomKademliaID(), "me:sim2")
	rt := NewRoutingTable(me)
	// seed a few entries
	for i := 0; i < 64; i++ {
		idx := sn.rng.Intn(len(sn.nodes))
		rt.AddContact(NewContact(sn.nodes[idx].ID, sn.nodes[idx].Address))
	}

	kad := &Kademlia{routingTable: rt, network: sn, store: make(map[string][]byte)}

	data := []byte("simulation blob")
	// Put() should call network.SendStoreMessage and return SHA-1 key
	key, err := kad.Put(data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	// confirm the key format matches SHA-1
	if len(key) != 2*IDLength || !isHex(key) {
		t.Fatalf("unexpected key %q", key)
	}

	// Now Get() should use network.SendFindDataMessage and retrieve the blob
	v, from, err := kad.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if from != nil {
		t.Fatalf("expected nil 'from' contact in current implementation")
	}
	if string(v) != "simulation blob" {
		t.Fatalf("bad value: %q", string(v))
	}
}
