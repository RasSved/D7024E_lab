package kademlia

import (
	"sync"
	"testing"
)

// ----------------- tests -------------------

func TestNewRoutingTable_InitializesBucketsAndMe(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	if rt == nil {
		t.Fatalf("routing table is nil")
	}
	if rt.me.ID == nil || rt.me.Address != "me" {
		t.Fatalf("me not set correctly: %#v", rt.me)
	}
	for i := 0; i < IDLength*8; i++ {
		if rt.buckets[i] == nil {
			t.Fatalf("bucket %d not initialized", i)
		}
	}
}

func TestGetBucketIndex_BitPositions_AndEqualIDs(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	// distance==0 => last bucket
	if got := rt.getBucketIndex(me.ID); got != IDLength*8-1 {
		t.Fatalf("equal IDs => want %d, got %d", IDLength*8-1, got)
	}

	// Single-bit targets should map to same bit index
	testBits := []int{0, 1, 7, 8, 31, 32, IDLength*8 - 2, IDLength*8 - 1}
	for _, bit := range testBits {
		id := idWithBit(t, bit)
		if got := rt.getBucketIndex(id); got != bit {
			t.Fatalf("bit %d => index %d, want %d", bit, got, bit)
		}
	}
}

func TestAddContact_AndFindClosestContacts_Smoke(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	c := mkContact(25, "c1", t)
	rt.AddContact(c)

	res := rt.FindClosestContacts(c.ID, 1)
	if len(res) != 1 || res[0].ID.String() != c.ID.String() {
		t.Fatalf("expected to find the same contact; got %#v", res)
	}
}

//func TestFindClosestContacts_SortedAndLimited(t *testing.T) {
//	me := NewContact(zeroID(t), "me")
//	rt := NewRoutingTable(me)
//
//	target := idWithBit(t, 40)
//
//	// Add contacts around target's bucket to ensure sweep + sorting
//	ids := []int{40, 41, 39, 0, IDLength*8 - 1, 100, 80, 38, 42}
//	for i, b := range ids {
//		rt.AddContact(mkContact(b, "n"+string('A'+i), t))
//	}
//
//	got := rt.FindClosestContacts(target, 5)
//	if len(got) != 5 {
//		t.Fatalf("want 5, got %d", len(got))
//	}
//
//	// Verify non-decreasing distance order
//	prev := got[0].ID.CalcDistance(target)
//	for i := 1; i < len(got); i++ {
//		d := got[i].ID.CalcDistance(target)
//		// compare big-endian lexicographically
//		for j := 0; j < IDLength; j++ {
//			if d[j] < prev[j] {
//				t.Fatalf("not sorted by distance at %d", i)
//			}
//			if d[j] > prev[j] {
//				break
//			}
//		}
//		prev = d
//	}
//
//	// The closest should be exact same bit (distance zero if same ID)
//	// We didn't insert target itself unless ids contains 40; it does.
//	if !got[0].ID.CalcDistance(target).Equals(zeroID(t)) {
//		t.Fatalf("expected exact target to be closest")
//	}
//}

func TestFindClosestContacts_UsesNeighborBuckets(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	// Empty target bucket; neighbors have entries
	target := idWithBit(t, 80)
	left := mkContact(79, "L", t)
	right := mkContact(81, "R", t)
	rt.AddContact(left)
	rt.AddContact(right)

	got := rt.FindClosestContacts(target, 2)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	seen := map[string]bool{got[0].ID.String(): true, got[1].ID.String(): true}
	if !seen[left.ID.String()] || !seen[right.ID.String()] {
		t.Fatalf("expected neighbors from buckets 79 and 81, got %#v", got)
	}
}

func TestFindClosestContacts_CountGreaterThanAvailable(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	// Add fewer than requested
	rt.AddContact(mkContact(5, "a", t))
	rt.AddContact(mkContact(6, "b", t))

	got := rt.FindClosestContacts(idWithBit(t, 6), 10)
	if len(got) != 2 {
		t.Fatalf("wanted all available=2, got %d", len(got))
	}
}

func TestAddContact_Concurrent_NoRace(t *testing.T) {
	me := NewContact(zeroID(t), "me")
	rt := NewRoutingTable(me)

	var wg sync.WaitGroup
	n := 200
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			rt.AddContact(mkContact(i%(IDLength*8), "x", t))
		}()
	}
	wg.Wait()

	// sanity: can query without panic and get something
	res := rt.FindClosestContacts(idWithBit(t, 0), 3)
	if len(res) == 0 {
		t.Fatalf("expected some contacts")
	}
}
