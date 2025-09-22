package kademlia

import "testing"

func TestKademliaID_CalcDistance(t *testing.T) {
	a := NewKademliaID("0000000000000000000000000000000000000001")
	b := NewKademliaID("0000000000000000000000000000000000000003")
	got := a.CalcDistance(b)

	// 0x01 XOR 0x03 = 0x02
	want := NewKademliaID("0000000000000000000000000000000000000002")
	if got.String() != want.String() {
		t.Fatalf("distance = %s, want %s", got.String(), want.String())
	}
}
