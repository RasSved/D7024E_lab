package kademlia

import (
	"container/list"
	"encoding/hex"
	"testing"
)

// --- Small helpers ---------------------------------------------------------

// Returns *KademliaID from a hex string of length IDLength*2.
// CHANGE this to your actual constructor if different.
func idFromHex(t *testing.T, h string) *KademliaID {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	if len(b) != IDLength {
		t.Fatalf("expected %d bytes, got %d", IDLength, len(b))
	}
	return NewKademliaID(hex.EncodeToString(b)) // <-- change if needed
}

func zeroID(t *testing.T) *KademliaID {
	t.Helper()
	return idFromHex(t, hex.EncodeToString(make([]byte, IDLength)))
}

// Create an ID with exactly one bit set at bitIdx (0 is MSB of byte 0).
func idWithBit(t *testing.T, bitIdx int) *KademliaID {
	t.Helper()
	if bitIdx < 0 || bitIdx >= IDLength*8 {
		t.Fatalf("bitIdx out of range: %d", bitIdx)
	}
	b := make([]byte, IDLength)
	byteIdx := bitIdx / 8
	shift := 7 - (bitIdx % 8)
	b[byteIdx] = 1 << uint(shift)
	return NewKademliaID(hex.EncodeToString(b)) // <-- change if needed
}

func mkContact(bit int, name string, t *testing.T) Contact {
	return NewContact(idWithBit(t, bit), name)
}

// --- Tests -----------------------------------------------------------------

func TestBucket_NewBucket_InitializesList(t *testing.T) {
	b := newBucket()
	if b == nil || b.list == nil {
		t.Fatalf("bucket/list should be initialized")
	}
	if b.list.Len() != 0 {
		t.Fatalf("new list should be empty")
	}
}

func TestBucket_AddContact_NewAndMoveToFront(t *testing.T) {
	b := newBucket()

	c1 := mkContact(8, "c1", t)
	c2 := mkContact(9, "c2", t)

	b.AddContact(c1)
	b.AddContact(c2)

	// New contacts go to front, so front must be c2, back must be c1
	if b.list.Front().Value.(Contact).Address != "c2" {
		t.Fatalf("expected front=c2, got %v", b.list.Front().Value.(Contact).Address)
	}
	if b.list.Back().Value.(Contact).Address != "c1" {
		t.Fatalf("expected back=c1, got %v", b.list.Back().Value.(Contact).Address)
	}

	// Adding c1 again should move it to front
	b.AddContact(c1)
	if b.list.Front().Value.(Contact).Address != "c1" {
		t.Fatalf("expected front=c1 after move-to-front")
	}

	// Ensure MoveToFront didn’t duplicate entries
	count := 0
	for e := b.list.Front(); e != nil; e = e.Next() {
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 unique contacts, got %d", count)
	}
}

func TestBucket_AddContact_RespectsCapacity_NoEviction(t *testing.T) {
	b := newBucket()
	// Fill to capacity
	for i := 0; i < bucketSize; i++ {
		b.AddContact(mkContact(16+i, "c"+string(rune('A'+i)), t))
	}
	if b.Len() != bucketSize {
		t.Fatalf("expected len=%d, got %d", bucketSize, b.Len())
	}

	// Add more; implementation ignores new ones (no eviction)
	before := snapshotList(b.list)
	b.AddContact(mkContact(999%(IDLength*8), "overflow", t))
	after := snapshotList(b.list)

	if b.Len() != bucketSize {
		t.Fatalf("overflow should not grow the list")
	}
	if !equalContacts(before, after) {
		t.Fatalf("expected identical list when at capacity")
	}
}

func TestBucket_GetContactAndCalcDistance_SetsDistanceAndOrder(t *testing.T) {
	b := newBucket()
	tgt := zeroID(t)

	// Insert in known order: c3 (front), c2, c1 (back)
	c1 := mkContact(1, "c1", t)
	c2 := mkContact(2, "c2", t)
	c3 := mkContact(3, "c3", t)
	b.AddContact(c1)
	b.AddContact(c2)
	b.AddContact(c3)

	contacts := b.GetContactAndCalcDistance(tgt)

	// Should preserve iteration order front..back: c3, c2, c1
	if len(contacts) != 3 ||
		contacts[0].Address != "c3" ||
		contacts[1].Address != "c2" ||
		contacts[2].Address != "c1" {
		t.Fatalf("unexpected order from GetContactAndCalcDistance: %#v", contacts)
	}

	// Their distance field should be usable (Less shouldn’t panic)
	// Sort a copy via ContactCandidates to ensure distances got set.
	cc := ContactCandidates{}
	cc.Append(contacts)
	cc.Sort()
	_ = cc.GetContacts(2) // smoke
}

// --- tiny helpers for list comparisons -------------------------------------

func snapshotList(l *list.List) []string {
	var out []string
	for e := l.Front(); e != nil; e = e.Next() {
		out = append(out, e.Value.(Contact).Address)
	}
	return out
}

func equalContacts(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
