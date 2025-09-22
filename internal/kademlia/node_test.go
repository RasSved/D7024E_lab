package kademlia

import (
	"testing"
)

// TestNewNode_NoBootstrap verifies that NewNode wires up the routing table,
// network, and "me" contact correctly, and that Close is idempotent.
func TestNewNode_NoBootstrap(t *testing.T) {
	// port 0 = let OS choose an ephemeral port; safe for unit tests
	node, err := NewNode("127.0.0.1", 0, "")
	if err != nil {
		t.Fatalf("NewNode returned error: %v", err)
	}
	if node == nil {
		t.Fatalf("NewNode returned nil node")
	}

	// Basic wiring checks
	if node.RoutingTable() == nil {
		t.Fatalf("RoutingTable should not be nil")
	}
	if node.Network() == nil {
		t.Fatalf("Network should not be nil")
	}

	// "me" contact should match the network address used
	me := node.RoutingTable().me
	if me.ID == nil || me.Address == "" {
		t.Fatalf("invalid 'me' contact in routing table: %#v", me)
	}
	if node.Address() == "" {
		t.Fatalf("node.Address() should return the network address")
	}
	if me.Address != node.Address() {
		t.Fatalf("me.Address %q != node.Address() %q", me.Address, node.Address())
	}

	// Close should succeed and be idempotent
	if err := node.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if err := node.Close(); err != nil {
		t.Fatalf("Close() second call should be nil, got: %v", err)
	}
}

// OPTIONAL: this exercises the bootstrap branch in NewNode.
// It will take ~2 seconds because iterativeLookupContact uses a 2s RPC timeout.
// Uncomment the Skip() to run it when you want the extra coverage.
//
// What it verifies:
// - bootstrap contact is added to the routing table during Join()
// - NewNode still returns a working node with bootstrap configured.
func TestNewNode_WithBootstrap(t *testing.T) {
	t.Skip("Unskip to run bootstrap path (slower ~2s due to rpcTimeout)")

	bootstrapAddr := "127.0.0.1:9999" // UDP send is connectionless; no listener is needed
	node, err := NewNode("127.0.0.1", 0, bootstrapAddr)
	if err != nil {
		t.Fatalf("NewNode (with bootstrap) error: %v", err)
	}
	defer node.Close()

	if node.RoutingTable() == nil || node.Network() == nil {
		t.Fatalf("expected non-nil RT and Network")
	}

	// After Join(), bootstrap contact should be present in the RT.
	// We can check by looking for a contact with Address == bootstrapAddr among closest to a random ID.
	found := false
	probe := NewRandomKademliaID()
	for _, c := range node.RoutingTable().FindClosestContacts(probe, k) {
		if c.Address == bootstrapAddr {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected bootstrap address %q to be present in routing table", bootstrapAddr)
	}
}
