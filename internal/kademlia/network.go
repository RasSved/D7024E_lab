package kademlia

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type MessageType byte

const (
	PING MessageType = iota
	PONG
	FIND_NODE
	FIND_NODE_RESPONSE
	STORE
	FIND_VALUE
	FIND_VALUE_RESPONSE
)

type PendingRequest struct {
	Timestamp    time.Time
	ResponseChan chan Message
	TargetID     *KademliaID
}

// Message represents a Kademlia RPC message with 160-bit RPC ID
type Message struct {
	RPCID  *KademliaID `json:"rpcid"`  // 160-bit RPC identifier
	Type   MessageType `json:"type"`   // Message type
	Sender Contact     `json:"sender"` // Senders contact info
	Target *KademliaID `json:"target"` // Target ID for FIND_NODE operation
	Data   []byte      `json:"data"`   // Data for STORE operations
	Nodes  []Contact   `json:"nodes"`  // Nodes for FIND_NODE_RESPONSE operation
}

// Network represents a Kademlia network node with IDP communication capabilities
type Network struct {
	nodeID          *KademliaID
	address         string
	conn            *net.UDPConn
	RoutingTable    *RoutingTable
	dataStore       map[string][]byte
	dataMutex       sync.RWMutex
	rpcMutex        sync.Mutex
	rpcCallbacks    map[string]chan RPCResponse // For handling async responses
	pendingRequests map[string]*PendingRequest
	pendingMutex    sync.Mutex
}

// RPCResponse represents a response from a remote procedure call
type RPCResponse struct {
	MessageType string
	Payload     []byte
	Error       error
}

// Listen creates and starts a Network instance listening on the specified IP and port
func Listen(ip string, port int) *Network {
	addrStr := net.JoinHostPort(ip, strconv.Itoa(port))

	// Set up UDP connection
	udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
	if err != nil {
		log.Fatal("Error resolving UDP address:", err)
	}
	log.Printf("Resolved UDP address: %s\n", udpAddr.String())

	// Create UDP listener
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal("Error listening on UDP:", err)
	}
	log.Printf("Listening on %s\n", addrStr)

	// Create node ID and contact info
	nodeID := NewRandomKademliaID()
	myContact := NewContact(nodeID, addrStr)
	log.Printf("Node ID: %s, Contact: %+v\n", nodeID.String(), myContact)

	// Initialize Network instance with all required components
	network := &Network{
		nodeID:          nodeID,
		address:         addrStr,
		conn:            conn,
		RoutingTable:    NewRoutingTable(myContact),
		dataStore:       make(map[string][]byte),
		rpcCallbacks:    make(map[string]chan RPCResponse),
		pendingRequests: make(map[string]*PendingRequest),
	}

	// Start background listener for incoming messages
	go network.listenLoop()

	return network
}

// serializeMessage converts a Message struct to JSON bytes for transmission
func (network *Network) serializeMessage(msg Message) ([]byte, error) {
	return json.Marshal(msg)
}

// deserializeMessage converts JSON bytes back to Message struct
func (network *Network) deserializeMessage(data []byte) (Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return msg, err
}

// listenLoop continuously listens for incoming UDP messages
func (network *Network) listenLoop() {
	buffer := make([]byte, 8192) //8KB buffer
	for {
		// Read data from UDP connection
		n, addr, err := network.conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Error reading from UDP: %v\n", err)
			continue
		}
		log.Printf("Received %d bytes from %s\n", n, addr.String())
		// Handle each message in its own goroutine for concurrency
		go network.handleMessage(buffer[:n], addr)
	}
}

// handleMessage processes incoming messages based on their type
func (network *Network) handleMessage(data []byte, addr *net.UDPAddr) {
	// Parse the message using proper deserialization
	msg, err := network.deserializeMessage(data)
	if err != nil {
		log.Printf("Error deserializing message: %v\n", err)
		return
	}

	// Update routing table with senders contact info
	network.RoutingTable.AddContact(msg.Sender)

	// Handle different message types
	switch msg.Type {
	case PING:
		network.handlePing(msg, addr)
	case PONG:
		network.handlePong(msg, addr)
	case FIND_NODE:
		network.handleFindNode(msg, addr)
	case FIND_NODE_RESPONSE:
		network.handleFindNodeResponse(msg, addr)
	case STORE:
		network.handleStore(msg, addr)
	case FIND_VALUE:
		network.handleFindValue(msg, addr)
	case FIND_VALUE_RESPONSE:
		network.handleFindValueResponse(msg, addr)
	default:
		log.Printf("Received unknown message type: %d\n", msg.Type)
	}
}

// handlePing processes PING requests and sends PONG responses
func (network *Network) handlePing(msg Message, addr *net.UDPAddr) {
	log.Printf("Received PING from %s (RPC ID: %s)\n", msg.Sender.Address, msg.RPCID.String())

	// Create PONG response with same RPC ID
	response := Message{
		RPCID:  msg.RPCID, // Use same RPC ID as request
		Type:   PONG,
		Sender: network.RoutingTable.me,
	}

	responseData, err := network.serializeMessage(response)
	if err != nil {
		log.Printf("Error serializing PONG response: %v\n", err)
		return
	}

	_, err = network.conn.WriteToUDP(responseData, addr)
	if err != nil {
		log.Printf("Error sending PONG response: %v\n", err)
	}
}

// handlePong processes PONG responses
func (network *Network) handlePong(msg Message, addr *net.UDPAddr) {
	log.Printf("Received PONG from %s (RPC ID: %s)\n", msg.Sender.Address, msg.RPCID.String())
	// TODO: Implement response handling for async requests
}

// handleFindNode processes FIND_NODE requests
func (network *Network) handleFindNode(msg Message, addr *net.UDPAddr) {
	log.Printf("Received FIND_NODE for target %s from %s\n", msg.Target.String(), msg.Sender.Address)

	// Find closest nodes to the target
	closestNodes := network.RoutingTable.FindClosestContacts(msg.Target, 20) // k=20

	// Send response
	response := Message{
		RPCID:  msg.RPCID, // Match request RPC ID
		Type:   FIND_NODE_RESPONSE,
		Sender: network.RoutingTable.me,
		Nodes:  closestNodes,
	}

	responseData, err := network.serializeMessage(response)
	if err != nil {
		log.Printf("Error serializing FIND_NODE_RESPONSE: %v\n", err)
		return
	}

	_, err = network.conn.WriteToUDP(responseData, addr)
	if err != nil {
		log.Printf("Error sending FIND_NODE_RESPONSE: %v\n", err)
	}
}

// handleFindNodeResponse processes FIND_NODE responses
func (network *Network) handleFindNodeResponse(msg Message, addr *net.UDPAddr) {
	log.Printf("Received FIND_NODE_RESPONSE with %d nodes from %s (RPC ID: %s)\n", len(msg.Nodes), msg.Sender.Address, msg.RPCID.String())

	// 1. Check if this is a response to a pending request
	network.pendingMutex.Lock()
	if pendingReq, exists := network.pendingRequests[msg.RPCID.String()]; exists {
		// Send response to waiting goroutine
		pendingReq.ResponseChan <- msg
		delete(network.pendingRequests, msg.RPCID.String())
		network.pendingMutex.Unlock()

		log.Printf("Found pending request for RPC ID %s - delivered response\n", msg.RPCID.String())
	} else {
		network.pendingMutex.Unlock()
		// IF no pending request, still process nodes for routing table
		for _, contact := range msg.Nodes {
			network.RoutingTable.AddContact(contact)
		}
		log.Printf("Added %d contacts from unsolicited FIND_NODE_RESPONSE\n", len(msg.Nodes))
	}
	//Always update senders contact info
	network.RoutingTable.AddContact(msg.Sender)
}

// SendPingMessage sends a PING message to the specified contact with 160-bit RPC ID
func (network *Network) SendPingMessage(contact *Contact) error {
	// Generate 160-bit RPC ID
	rpcID := NewRandomKademliaID()

	// Create PING message with RPC ID
	msg := Message{
		RPCID:  rpcID, //160-bit RPC identifier
		Type:   PING,
		Sender: network.RoutingTable.me,
	}

	//Serialize message
	msgData, err := network.serializeMessage(msg)
	if err != nil {
		return err
	}

	// Resolve the target address
	udpAddr, err := net.ResolveUDPAddr("udp", contact.Address)
	if err != nil {
		return err
	}

	// Send PING message via UDP
	_, err = network.conn.WriteToUDP(msgData, udpAddr)
	if err != nil {
		return err
	}

	log.Printf("Sent PING to %s (RPC ID: %s)\n", contact.Address, rpcID.String())
	return nil
}

// SendFindContactMessage sends a FIND_NODE message to find contacts close to target ID
func (network *Network) SendFindContactMessage(contact *Contact, targetID *KademliaID) (<-chan []Contact, error) {
	// Generate 160-bit RPC ID
	rpcID := NewRandomKademliaID()

	// Create response channel
	respChan := make(chan Message, 1)

	// Store pending request
	network.pendingMutex.Lock()
	network.pendingRequests[rpcID.String()] = &PendingRequest{
		Timestamp:    time.Now(),
		ResponseChan: respChan,
		TargetID:     targetID,
	}
	network.pendingMutex.Unlock()

	// Create FIND_NODE message
	msg := Message{
		RPCID:  rpcID, // 160-bit RPC identifier
		Type:   FIND_NODE,
		Sender: network.RoutingTable.me,
		Target: targetID, // Target node to find
	}

	// Serialize and send message (similar to SendPingMessage)
	msgData, err := network.serializeMessage(msg)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, err
	}

	udpAddr, err := net.ResolveUDPAddr("udp", contact.Address)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, err
	}

	_, err = network.conn.WriteToUDP(msgData, udpAddr)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, err
	}

	log.Printf("Sent FIND_NODE to %s for target %s (RPC ID: %s)\n", contact.Address, targetID.String(), rpcID.String())

	//Wait for response with timeout
	select {
	case response := <-respChan:
		return response.Nodes, nil
	case <-time.After(5 * time.Second):
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, errors.New("FIND_NODE request timeout")
	}
}

func (network *Network) SendFindDataMessage(hash string) {
	// TODO: Implement with proper message format
}

func (network *Network) SendStoreMessage(data []byte) {
	// TODO: Implement with proper message format
}

// Think we need this
func NewNetwork(me *Contact, rt *RoutingTable) *Network {
	// TODO: open UDP socket, set fields, start receiver goroutine, etc.
	return &Network{
		RoutingTable: rt,
		// conn: ..., etc.
	}
}
