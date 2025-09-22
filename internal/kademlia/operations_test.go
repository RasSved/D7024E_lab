package kademlia

import (
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

// ---------- fake network (implements NetworkAPI) ----------

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

var ErrFake = fakeErr("fake error")

type opsFakeNetwork struct {
	addr        string
	pingCount   int
	storeCount  int
	lastStored  []byte
	findData    map[string][]byte
	findDataErr bool
}

func (f *opsFakeNetwork) SendFindContactMessageAsync(peer *Contact, target *KademliaID) (<-chan []Contact, error) {
	ch := make(chan []Contact)
	close(ch)
	return ch, nil
}
func (f *opsFakeNetwork) SendPingMessage(*Contact) error { f.pingCount++; return nil }
func (f *opsFakeNetwork) SendStoreMessage(data []byte) error {
	f.storeCount++
	f.lastStored = append([]byte(nil), data...)
	return nil
}
func (f *opsFakeNetwork) SendFindDataMessage(hash string) ([]byte, error) {
	if f.findDataErr {
		return nil, fakeErr("forced error")
	}
	if d, ok := f.findData[hash]; ok {
		return append([]byte(nil), d...), nil
	}
	return nil, fakeErr("not found")
}
func (f *opsFakeNetwork) Address() string { return f.addr }
func (f *opsFakeNetwork) Close() error    { return nil }

// ---------- tests ----------

func TestJoin_WithBootstrap_AddsContactAndPings(t *testing.T) {
	me := NewContact(zeroID(t), "me:0")
	rt := NewRoutingTable(me)

	fnet := &opsFakeNetwork{addr: "node:1"}
	k := &Kademlia{
		routingTable: rt,
		network:      fnet,
		store:        make(map[string][]byte),
	}

	bootstrap := NewContact(NewRandomKademliaID(), "127.0.0.1:9999")
	k.Join(&bootstrap)

	// bootstrap should have been added to RT
	closest := k.routingTable.FindClosestContacts(bootstrap.ID, 1)
	found := false
	for _, c := range closest {
		if c.Address == bootstrap.Address {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected bootstrap %q to be in routing table", bootstrap.Address)
	}

	// and a ping should have been sent
	if fnet.pingCount == 0 {
		t.Fatalf("expected SendPingMessage to be called at least once")
	}
}

func TestJoin_NoBootstrap_NoPanic(t *testing.T) {
	me := NewContact(zeroID(t), "me:0")
	rt := NewRoutingTable(me)
	k := &Kademlia{routingTable: rt, network: &fakeNetwork{}, store: make(map[string][]byte)}
	k.Join(nil) // should simply be best-effort and return
}

func TestPut_WithNetwork_UsesNetworkAndReturnsSHA1(t *testing.T) {
	me := NewContact(zeroID(t), "me:0")
	rt := NewRoutingTable(me)

	fnet := &opsFakeNetwork{}
	k := &Kademlia{routingTable: rt, network: fnet, store: make(map[string][]byte)}

	data := []byte("hello world")
	wantKey := sha1Hex(data)

	gotKey, err := k.Put(data)
	if err != nil {
		t.Fatalf("Put error: %v", err)
	}
	if gotKey != wantKey {
		t.Fatalf("wrong key: got %s want %s", gotKey, wantKey)
	}
	if fnet.storeCount != 1 {
		t.Fatalf("expected network SendStoreMessage to be called once, got %d", fnet.storeCount)
	}
	// local store should remain empty in network path
	if len(k.store) != 0 {
		t.Fatalf("expected no local storage when network is present")
	}
	// and fake stored a copy
	data[0] = 'H'
	if string(fnet.lastStored) != "hello world" {
		t.Fatalf("network should have received original data copy")
	}
}

//func TestPut_Offline_StoresLocally_Copy(t *testing.T) {
//	me := NewContact(zeroID(t), "me:0")
//	rt := NewRoutingTable(me)
//
//	k := &Kademlia{routingTable: rt, network: nil, store: make(map[string][]byte)}
//	data := []byte("abc")
//	wantKey := sha1Hex(data)
//
//	gotKey, err := k.Put(data)
//	if err != nil {
//		t.Fatalf("Put error: %v", err)
//	}
//	if gotKey != wantKey {
//		t.Fatalf("wrong key: got %s want %s", gotKey, wantKey)
//	}
//
//	// ensure stored copy (mutating input doesn't affect stored value)
//	data[0] = 'X'
//	got, _, err := k.Get(gotKey)
//	if err != nil || string(got) != "abc" {
//		t.Fatalf("expected local value 'abc', got %q, err=%v", string(got), err)
//	}
//}

func TestGet_WithNetworkHit(t *testing.T) {
	me := NewContact(zeroID(t), "me:0")
	rt := NewRoutingTable(me)

	// prepare network to return data for key
	payload := []byte("netdata")
	key := sha1Hex(payload)

	fnet := &opsFakeNetwork{findData: map[string][]byte{key: payload}}
	k := &Kademlia{routingTable: rt, network: fnet, store: make(map[string][]byte)}

	got, from, err := k.Get(key)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if from != nil {
		t.Fatalf("Get currently should return nil 'from' contact")
	}
	if string(got) != "netdata" {
		t.Fatalf("expected network value, got %q", string(got))
	}
	// returned slice should be a copy
	got[0] = 'N'
	got2, _, _ := k.Get(key)
	if string(got2) != "netdata" {
		t.Fatalf("network result should be immutable from caller")
	}
}

//func TestGet_WithNetworkMiss_FallsBackToLocalCopy(t *testing.T) {
//	me := NewContact(zeroID(t), "me:0")
//	rt := NewRoutingTable(me)
//
//	// network returns error/miss
//	fnet := &opsFakeNetwork{findDataErr: true}
//	k := &Kademlia{routingTable: rt, network: fnet, store: make(map[string][]byte)}
//
//	// put locally (simulate offline store)
//	local := []byte("local")
//	key := sha1Hex(local)
//	k.store[key] = append([]byte(nil), local...)
//
//	got, from, err := k.Get(key)
//	if err != nil {
//		t.Fatalf("Get should fall back to local without error, got: %v", err)
//	}
//	if from != nil {
//		t.Fatalf("expected nil 'from' on local fallback")
//	}
//	if string(got) != "local" {
//		t.Fatalf("expected local value, got %q", string(got))
//	}
//	// ensure copy-on-read
//	got[0] = 'L'
//	got2, _, _ := k.Get(key)
//	if string(got2) != "local" {
//		t.Fatalf("local store mutated via returned slice")
//	}
//}

// ---------- tiny util ----------

func sha1Hex(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}
