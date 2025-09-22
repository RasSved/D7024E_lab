package kademlia

import (
	"encoding/hex"
	"fmt"
	"net"
	"testing"
	"time"
)

func newBareNetworkForUnit(t *testing.T) *Network {
	t.Helper()
	me := NewContact(NewRandomKademliaID(), "me:0")
	return &Network{
		nodeID:          me.ID,
		address:         "unused",
		conn:            nil, // no real UDP
		RoutingTable:    NewRoutingTable(me),
		dataStore:       make(map[string][]byte),
		rpcCallbacks:    make(map[string]chan RPCResponse),
		pendingRequests: make(map[string]*PendingRequest),
		done:            make(chan struct{}),
	}
}

func TestSerializeDeserialize_RoundTrip(t *testing.T) {
	n := newBareNetworkForUnit(t)
	rpc := NewRandomKademliaID()
	msg := Message{
		RPCID:  rpc,
		Type:   FIND_NODE_RESPONSE,
		Sender: n.RoutingTable.me,
		Nodes:  []Contact{NewContact(NewRandomKademliaID(), "x:1")},
	}
	b, err := n.serializeMessage(msg)
	if err != nil {
		t.Fatalf("serialize error: %v", err)
	}
	got, err := n.deserializeMessage(b)
	if err != nil {
		t.Fatalf("deserialize error: %v", err)
	}
	if got.Type != FIND_NODE_RESPONSE || got.RPCID.String() != rpc.String() {
		t.Fatalf("round trip mismatch")
	}
}

func TestHashData_SHA1HexLen(t *testing.T) {
	n := newBareNetworkForUnit(t)
	h := n.hashData([]byte("hello"))
	if len(h) != 2*IDLength { // 20 bytes -> 40 hex chars for SHA-1
		t.Fatalf("unexpected hex length: %d", len(h))
	}
	if _, err := hex.DecodeString(h); err != nil {
		t.Fatalf("not valid hex: %v", err)
	}
}

func TestHandleFindNodeResponse_DeliversPendingAndUpdatesRT(t *testing.T) {
	n := newBareNetworkForUnit(t)

	rpc := NewRandomKademliaID()
	respCh := make(chan Message, 1)
	resNodes := []Contact{
		NewContact(NewRandomKademliaID(), "a:1"),
		NewContact(NewRandomKademliaID(), "b:2"),
	}

	n.pendingMutex.Lock()
	n.pendingRequests[rpc.String()] = &PendingRequest{
		Timestamp:    time.Now(),
		ResponseChan: respCh,
		TargetID:     NewRandomKademliaID(),
	}
	n.pendingMutex.Unlock()

	sender := NewContact(NewRandomKademliaID(), "sender:9")
	msg := Message{
		RPCID:  rpc,
		Type:   FIND_NODE_RESPONSE,
		Sender: sender,
		Nodes:  resNodes,
	}

	n.handleFindNodeResponse(msg, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1})

	// response delivered
	select {
	case m := <-respCh:
		if len(m.Nodes) != len(resNodes) {
			t.Fatalf("expected %d nodes, got %d", len(resNodes), len(m.Nodes))
		}
	default:
		t.Fatalf("no response delivered to pending")
	}

	// pending cleared
	n.pendingMutex.Lock()
	_, still := n.pendingRequests[rpc.String()]
	n.pendingMutex.Unlock()
	if still {
		t.Fatalf("pending not cleared")
	}

	// sender added to RT
	closest := n.RoutingTable.FindClosestContacts(sender.ID, 1)
	if len(closest) == 0 || closest[0].ID.String() != sender.ID.String() {
		t.Fatalf("sender not added to routing table")
	}
}

func TestHandleFindValueResponse_DeliversPending(t *testing.T) {
	n := newBareNetworkForUnit(t)

	rpc := NewRandomKademliaID()
	respCh := make(chan Message, 1)

	n.pendingMutex.Lock()
	n.pendingRequests[rpc.String()] = &PendingRequest{
		Timestamp:    time.Now(),
		ResponseChan: respCh,
		TargetID:     NewRandomKademliaID(),
	}
	n.pendingMutex.Unlock()

	data := []byte("payload")
	msg := Message{
		RPCID:  rpc,
		Type:   FIND_VALUE_RESPONSE,
		Sender: n.RoutingTable.me,
		Data:   data,
	}

	n.handleFindValueResponse(msg, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2})

	select {
	case m := <-respCh:
		if string(m.Data) != "payload" {
			t.Fatalf("wrong data returned: %q", string(m.Data))
		}
	default:
		t.Fatalf("no response delivered")
	}

	n.pendingMutex.Lock()
	_, still := n.pendingRequests[rpc.String()]
	n.pendingMutex.Unlock()
	if still {
		t.Fatalf("pending not cleared")
	}
}

func TestHandleMessage_RoutesFindNodeResponse(t *testing.T) {
	n := newBareNetworkForUnit(t)

	// prepare pending for RPC
	rpc := NewRandomKademliaID()
	respCh := make(chan Message, 1)
	n.pendingMutex.Lock()
	n.pendingRequests[rpc.String()] = &PendingRequest{
		Timestamp:    time.Now(),
		ResponseChan: respCh,
		TargetID:     NewRandomKademliaID(),
	}
	n.pendingMutex.Unlock()

	msg := Message{
		RPCID:  rpc,
		Type:   FIND_NODE_RESPONSE,
		Sender: NewContact(NewRandomKademliaID(), "s:1"),
		Nodes:  []Contact{NewContact(NewRandomKademliaID(), "x:2")},
	}
	raw, _ := n.serializeMessage(msg)

	// This calls handleMessage -> handleFindNodeResponse
	n.handleMessage(raw, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3})

	select {
	case <-respCh:
		// ok
	default:
		t.Fatalf("no delivery via handleMessage")
	}
}

func TestHandlePing_SendsPongWithSameRPCID(t *testing.T) {
	n := newBareNetworkForUnit(t)

	// sender socket to write responses
	sock, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen sender: %v", err)
	}
	defer sock.Close()
	n.conn = sock

	// receiver socket to capture PONG
	rcv, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen recv: %v", err)
	}
	defer rcv.Close()

	rpc := NewRandomKademliaID()
	req := Message{
		RPCID:  rpc,
		Type:   PING,
		Sender: n.RoutingTable.me,
	}
	n.handlePing(req, rcv.LocalAddr().(*net.UDPAddr))

	_ = rcv.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 4096)
	nread, _, err := rcv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read PONG: %v", err)
	}

	got, err := n.deserializeMessage(buf[:nread])
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if got.Type != PONG || got.RPCID.String() != rpc.String() {
		t.Fatalf("want PONG with same RPCID, got %+v", got)
	}
}

func TestHandleFindNode_SendsClosestNodes(t *testing.T) {
	n := newBareNetworkForUnit(t)
	// seed routing table with a few nodes
	for i := 0; i < 5; i++ {
		n.RoutingTable.AddContact(NewContact(NewRandomKademliaID(), fmt.Sprintf("n:%d", i)))
	}

	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock

	rcv, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer rcv.Close()

	target := NewRandomKademliaID()
	req := Message{
		RPCID:  NewRandomKademliaID(),
		Type:   FIND_NODE,
		Sender: n.RoutingTable.me,
		Target: target,
	}
	n.handleFindNode(req, rcv.LocalAddr().(*net.UDPAddr))

	_ = rcv.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 8192)
	nread, _, err := rcv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read FN_RESP: %v", err)
	}

	resp, err := n.deserializeMessage(buf[:nread])
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if resp.Type != FIND_NODE_RESPONSE || len(resp.Nodes) == 0 {
		t.Fatalf("expected FIND_NODE_RESPONSE with nodes, got %+v", resp)
	}
}

func TestHandleFindValue_DataHit_ReturnsData(t *testing.T) {
	n := newBareNetworkForUnit(t)

	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock
	rcv, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer rcv.Close()

	key := NewRandomKademliaID()
	payload := []byte("hi")
	n.dataMutex.Lock()
	n.dataStore[key.String()] = payload
	n.dataMutex.Unlock()

	req := Message{RPCID: NewRandomKademliaID(), Type: FIND_VALUE, Sender: n.RoutingTable.me, Target: key}
	n.handleFindValue(req, rcv.LocalAddr().(*net.UDPAddr))

	_ = rcv.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 4096)
	nread, _, err := rcv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	resp, err := n.deserializeMessage(buf[:nread])
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if resp.Type != FIND_VALUE_RESPONSE || string(resp.Data) != "hi" {
		t.Fatalf("expected data in response, got %+v", resp)
	}
}

func TestHandleFindValue_Miss_ReturnsClosestNodes(t *testing.T) {
	n := newBareNetworkForUnit(t)
	// seed some peers so the miss path can include nodes
	for i := 0; i < 3; i++ {
		n.RoutingTable.AddContact(NewContact(NewRandomKademliaID(), fmt.Sprintf("p:%d", i)))
	}

	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock
	rcv, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer rcv.Close()

	key := NewRandomKademliaID()
	req := Message{RPCID: NewRandomKademliaID(), Type: FIND_VALUE, Sender: n.RoutingTable.me, Target: key}
	n.handleFindValue(req, rcv.LocalAddr().(*net.UDPAddr))

	_ = rcv.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 4096)
	nread, _, err := rcv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	resp, err := n.deserializeMessage(buf[:nread])
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if resp.Type != FIND_VALUE_RESPONSE || len(resp.Nodes) == 0 {
		t.Fatalf("expected nodes in miss response, got %+v", resp)
	}
}

func TestHandleStore_PersistsDataAndUpdatesRT(t *testing.T) {
	n := newBareNetworkForUnit(t)

	key := NewRandomKademliaID()
	msg := Message{
		RPCID:  NewRandomKademliaID(),
		Type:   STORE,
		Sender: NewContact(NewRandomKademliaID(), "peer:1"),
		Target: key,
		Data:   []byte("blob"),
	}
	n.handleStore(msg, &net.UDPAddr{}) // addr unused here

	n.dataMutex.RLock()
	got := n.dataStore[key.String()]
	n.dataMutex.RUnlock()
	if string(got) != "blob" {
		t.Fatalf("data not stored")
	}
	// sender should be in RT
	closest := n.RoutingTable.FindClosestContacts(msg.Sender.ID, 1)
	if len(closest) == 0 || closest[0].ID.String() != msg.Sender.ID.String() {
		t.Fatalf("sender not added to RT")
	}
}

// Async: we intercept the rpcID from pendingRequests and inject a response.
func TestSendFindContactMessageAsync_HappyPath(t *testing.T) {
	n := newBareNetworkForUnit(t)
	// socket required for WriteToUDP to succeed
	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock

	peer := NewContact(NewRandomKademliaID(), "127.0.0.1:9999") // no listener needed for UDP write
	target := NewRandomKademliaID()

	ch, err := n.SendFindContactMessageAsync(&peer, target)
	if err != nil {
		t.Fatalf("async: %v", err)
	}

	// fetch the only pending rpcID
	n.pendingMutex.Lock()
	var rpcStr string
	for k := range n.pendingRequests {
		rpcStr = k
		break
	}
	n.pendingMutex.Unlock()
	if rpcStr == "" {
		t.Fatalf("no pending request")
	}

	// inject response via handler
	nodes := []Contact{NewContact(NewRandomKademliaID(), "x:1")}
	msg := Message{RPCID: NewKademliaID(rpcStr), Type: FIND_NODE_RESPONSE, Sender: peer, Nodes: nodes}
	n.handleFindNodeResponse(msg, &net.UDPAddr{})

	select {
	case got := <-ch:
		if len(got) != 1 || got[0].Address != "x:1" {
			t.Fatalf("unexpected nodes: %+v", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("no async result delivered")
	}
}

// Sync: same idea but the method blocks until we inject the response.
func TestSendFindContactMessage_HappyPath(t *testing.T) {
	n := newBareNetworkForUnit(t)
	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock

	peer := NewContact(NewRandomKademliaID(), "127.0.0.1:9998")
	target := NewRandomKademliaID()

	done := make(chan []Contact, 1)
	go func() {
		nodes, err := n.SendFindContactMessage(&peer, target)
		if err != nil {
			t.Errorf("sync: %v", err)
		}
		done <- nodes
	}()

	// wait until pending shows up
	var rpcStr string
	dead := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(dead) {
		n.pendingMutex.Lock()
		for k := range n.pendingRequests {
			rpcStr = k
			break
		}
		n.pendingMutex.Unlock()
		if rpcStr != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rpcStr == "" {
		t.Fatalf("no pending request observed")
	}

	// inject response
	nodes := []Contact{NewContact(NewRandomKademliaID(), "y:2")}
	msg := Message{RPCID: NewKademliaID(rpcStr), Type: FIND_NODE_RESPONSE, Sender: peer, Nodes: nodes}
	n.handleFindNodeResponse(msg, &net.UDPAddr{})

	select {
	case got := <-done:
		if len(got) != 1 || got[0].Address != "y:2" {
			t.Fatalf("unexpected nodes: %+v", got)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("sync result not delivered")
	}
}

func TestSendFindDataMessage_LocalHit(t *testing.T) {
	n := newBareNetworkForUnit(t)
	key := NewRandomKademliaID().String()
	n.dataMutex.Lock()
	n.dataStore[key] = []byte("loc")
	n.dataMutex.Unlock()
	got, err := n.SendFindDataMessage(key)
	if err != nil || string(got) != "loc" {
		t.Fatalf("expected local hit, got %q err=%v", string(got), err)
	}
}

func TestSendFindDataMessage_NoNodesAvailable(t *testing.T) {
	n := newBareNetworkForUnit(t)
	// empty RT means no nodes
	key := NewRandomKademliaID().String()
	if _, err := n.SendFindDataMessage(key); err == nil {
		t.Fatalf("expected 'no nodes available for lookup'")
	}
}

func TestSendStoreMessage_NoNodes_Error(t *testing.T) {
	n := newBareNetworkForUnit(t)
	if err := n.SendStoreMessage([]byte("x")); err == nil {
		t.Fatalf("expected error when RT has no nodes")
	}
}

func TestSendStoreMessage_SucceedsWithNodes(t *testing.T) {
	n := newBareNetworkForUnit(t)

	// seed some nodes
	for i := 0; i < 3; i++ {
		n.RoutingTable.AddContact(NewContact(NewRandomKademliaID(), fmt.Sprintf("h:%d", i)))
	}

	// a real UDP socket is required for WriteToUDP to succeed
	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock

	if err := n.SendStoreMessage([]byte("blob")); err != nil {
		t.Fatalf("SendStoreMessage: %v", err)
	}
}

func TestSendStoreToNode_WritesUDP(t *testing.T) {
	n := newBareNetworkForUnit(t)
	// sender and a random receiver addr
	sock, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer sock.Close()
	n.conn = sock

	rcv, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer rcv.Close()

	key := NewRandomKademliaID()
	target := NewContact(NewRandomKademliaID(), rcv.LocalAddr().String())
	if err := n.sendStoreToNode([]byte("x"), key, &target); err != nil {
		t.Fatalf("sendStoreToNode: %v", err)
	}
}
