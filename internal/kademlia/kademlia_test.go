package kademlia

import (
	"fmt"
	"strings"
	"testing"
)

// ------------------------------- Fake network --------------------------------

// Satisfies NetworkAPI
type fakeNetwork struct {
	addr    string
	replies map[string][]Contact // peerID -> nodes to return
	fail    map[string]bool      // peerID -> force error from SendFindContactMessageAsync
	data    map[string][]byte    // optional: for SendFindDataMessage
}

func (f *fakeNetwork) SendFindContactMessageAsync(peer *Contact, target *KademliaID) (<-chan []Contact, error) {
	if peer == nil || peer.ID == nil {
		ch := make(chan []Contact, 1)
		close(ch)
		return ch, nil
	}
	if f.fail != nil && f.fail[peer.ID.String()] {
		return nil, fmt.Errorf("send failed")
	}
	ch := make(chan []Contact, 1)
	ch <- f.replies[peer.ID.String()]
	close(ch)
	return ch, nil
}
func (f *fakeNetwork) SendPingMessage(*Contact) error { return nil }
func (f *fakeNetwork) SendStoreMessage([]byte) error  { return nil }
func (f *fakeNetwork) SendFindDataMessage(key string) ([]byte, error) {
	if f.data != nil {
		if d, ok := f.data[key]; ok {
			return append([]byte(nil), d...), nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeNetwork) Address() string { return f.addr }
func (f *fakeNetwork) Close() error    { return nil }

// ----------------------------------- Tests -----------------------------------

func TestLookupContact_HappyPath(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)
	rt.AddContact(mkContact(20, "c1", t))
	rt.AddContact(mkContact(21, "c2", t))
	rt.AddContact(mkContact(22, "c3", t))

	kad := &Kademlia{routingTable: rt}

	target := NewContact(idWithBit(t, 21), "target")
	got := kad.LookupContact(&target)
	if len(got) == 0 {
		t.Fatalf("expected non-empty result")
	}
}

func TestLookupData_HitAndCopy(t *testing.T) {
	kad := &Kademlia{store: make(map[string][]byte)}
	ok := kad.Store("abc", []byte("hello"))
	if !ok {
		t.Fatalf("store failed")
	}
	v, contacts := kad.LookupData("abc")
	if contacts != nil {
		t.Fatalf("hit should not return contacts")
	}
	if string(v) != "hello" {
		t.Fatalf("expected hello, got %q", string(v))
	}
	// ensure copy-on-read
	v[0] = 'J'
	v2, _ := kad.LookupData("abc")
	if string(v2) != "hello" {
		t.Fatalf("internal mutated")
	}
}

func TestLookupData_MissReturnsClosest(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)
	rt.AddContact(mkContact(5, "c5", t))
	rt.AddContact(mkContact(6, "c6", t))

	kad := &Kademlia{routingTable: rt, store: map[string][]byte{}}
	data, contacts := kad.LookupData(strings.Repeat("00", IDLength))
	if data != nil || len(contacts) == 0 {
		t.Fatalf("miss should return nil data and non-empty contacts")
	}
}

func TestIterativeLookupContact_WithRepliesAndDedup(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	pA := mkContact(50, "A", t)
	pB := mkContact(51, "B", t)
	pC := mkContact(52, "C", t) // will fail send
	rt.AddContact(pA)
	rt.AddContact(pB)
	rt.AddContact(pC)

	targetID := idWithBit(t, 40)
	target := NewContact(targetID, "target")

	nClose := NewContact(idWithBit(t, 41), "Nclose")
	nExact := NewContact(idWithBit(t, 40), "Nexact")

	replies := map[string][]Contact{
		pA.ID.String(): {nClose, nExact},
		pB.ID.String(): {nClose}, // duplicate should be de-duped
	}
	fail := map[string]bool{pC.ID.String(): true}

	net := &fakeNetwork{addr: "127.0.0.1:9000", replies: replies, fail: fail}
	kad := &Kademlia{routingTable: rt, network: net}

	res := kad.iterativeLookupContact(&target)
	if len(res) == 0 {
		t.Fatalf("expected non-empty shortlist")
	}
	// zero distance should be first
	if !res[0].ID.CalcDistance(targetID).Equals(NewKademliaID(strings.Repeat("00", IDLength))) {
		t.Fatalf("expected exact target to be closest")
	}
	// de-dup
	seen := map[string]bool{}
	for _, c := range res {
		if seen[c.ID.String()] {
			t.Fatalf("duplicate contact present: %s", c.ID.String())
		}
		seen[c.ID.String()] = true
	}
}

func TestMergeKeepBestKNoSort_DedupAndCap(t *testing.T) {
	target := zeroID(t)
	a := []Contact{
		NewContact(idWithBit(t, 60), "a1"),
		NewContact(idWithBit(t, 61), "a2"),
	}
	b := []Contact{
		NewContact(idWithBit(t, 61), "dup"),
		NewContact(idWithBit(t, 10), "new"),
	}
	added := mergeKeepBestKNoSort(&a, b, 3, func(c *Contact) { c.CalcDistance(target) })
	if !added || len(a) != 3 {
		t.Fatalf("expected one new contact; got len=%d added=%v", len(a), added)
	}

	// Make 'a' huge
	for i := 0; i < 50; i++ {
		a = append(a, NewContact(idWithBit(t, (i%(IDLength*8))), "x"))
	}

	// Trigger the function again with NON-EMPTY b to hit the cap branch.
	// It can even be a duplicate; cap runs as long as len(b) > 0.
	_ = mergeKeepBestKNoSort(&a, []Contact{NewContact(idWithBit(t, 120), "trigger")}, 2, nil)

	if len(a) > 4 { // 2 * ksize
		t.Fatalf("expected cap to <= 2*k, got %d", len(a))
	}
}

func TestKClosestAllQueried(t *testing.T) {
	c0 := NewContact(idWithBit(t, 1), "c0")
	c1 := NewContact(idWithBit(t, 2), "c1")
	short := []Contact{c0, c1}
	q := map[string]bool{c0.ID.String(): true}
	if kClosestAllQueried(short, q, 2) {
		t.Fatalf("not all queried yet")
	}
	q[c1.ID.String()] = true
	if !kClosestAllQueried(short, q, 2) {
		t.Fatalf("now all should be queried")
	}
}

func TestAccessors(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)
	net := &fakeNetwork{addr: "addr:1"}

	k := &Kademlia{routingTable: rt, network: net}
	if k.RoutingTable() == nil || k.Network() == nil {
		t.Fatalf("expected non-nil accessors")
	}
	if k.Address() != "addr:1" {
		t.Fatalf("unexpected address %q", k.Address())
	}
	k.network = nil
	if k.Address() != "" {
		t.Fatalf("expected empty address when network is nil")
	}
}
