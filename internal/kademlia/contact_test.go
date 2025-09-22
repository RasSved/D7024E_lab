package kademlia

import (
	"encoding/hex"
	"testing"
)

// --- helpers (same constructor caveat as above) ----------------------------

func idFromHex2(t *testing.T, h string) *KademliaID {
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

func zeros(t *testing.T) *KademliaID {
	return idFromHex2(t, hex.EncodeToString(make([]byte, IDLength)))
}

func ones(t *testing.T) *KademliaID {
	b := make([]byte, IDLength)
	for i := range b {
		b[i] = 0xFF
	}
	return NewKademliaID(hex.EncodeToString(b)) // <-- change if needed
}

// --- Tests -----------------------------------------------------------------

func TestNewContact_AndString(t *testing.T) {
	id := zeros(t)
	c := NewContact(id, "127.0.0.1:8000")
	s := c.String()
	if len(s) == 0 {
		t.Fatalf("String() should not be empty")
	}
	if c.ID != id || c.Address != "127.0.0.1:8000" {
		t.Fatalf("NewContact fields not set correctly")
	}
}

func TestContact_CalcDistance_AndLess(t *testing.T) {
	a := NewContact(zeros(t), "A")
	b := NewContact(ones(t), "B")

	target := zeros(t)

	// distance(a, target) == 0…0
	a.CalcDistance(target)
	// distance(b, target) == 1…1
	b.CalcDistance(target)

	if !a.Less(&b) {
		t.Fatalf("a should be closer (smaller distance) than b")
	}
	if b.Less(&a) {
		t.Fatalf("b should not be closer than a")
	}
}

func TestContactCandidates_Append_Sort_GetContacts(t *testing.T) {
	target := zeros(t)

	// Make three contacts at increasing distances from target.
	c0 := NewContact(zeros(t), "c0")
	c1 := NewContact(idFromHex2(t, "80"+makeZerosHex(IDLength-1)), "c1") // MSB set -> farther than zero
	c2 := NewContact(ones(t), "c2")                                      // farthest

	for _, c := range []*Contact{&c0, &c1, &c2} {
		c.CalcDistance(target)
	}

	var cc ContactCandidates
	cc.Append([]Contact{c2, c0}) // out of order on purpose
	cc.Append([]Contact{c1})

	cc.Sort()
	got := cc.GetContacts(2)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	// After sort by distance: c0 (closest), then c1 (middle)
	if got[0].Address != "c0" || got[1].Address != "c1" {
		t.Fatalf("unexpected order after sort: %#v", got)
	}
}

// helper to build a hex string of N zero bytes
func makeZerosHex(n int) string {
	b := make([]byte, n)
	return hex.EncodeToString(b)
}
