package kademlia

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"strconv"
	"strings"
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
	resultChan   chan []Contact // For async requests
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
	done            chan struct{}
}

// RPCResponse represents a response from a remote procedure call
type RPCResponse struct {
	MessageType string
	Payload     []byte
	Error       error
}

/*
*
*
*
*
*
*
*
*
*
*
 */

// Creates and Starts a Network instance listening on the specified IP and port
func Listen(ip string, port int) *Network {
	reqAddr := net.JoinHostPort(ip, strconv.Itoa(port))

	// Sets up UDP connection
	udpAddr, err := net.ResolveUDPAddr("udp", reqAddr)
	if err != nil {
		log.Fatal("Error resolving UDP address:", err)
	}
	log.Printf("Resolved UDP address: %s\n", udpAddr.String())

	// Creates a UDP listener
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal("Error listening on UDP:", err)
	}

	local := conn.LocalAddr().(*net.UDPAddr)
	addrStr := local.String()
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
		done:            make(chan struct{}),
	}

	// Starts background listener for incoming messages
	go network.listenLoop()
	// Returns a ready to use Network instance
	return network
}

// Alternative way to create a Network instance
func NewNetwork(ip string, port int) *Network {
	return Listen(ip, port)
}

// Uses JSON encoding to create a byte array that can be sent over UDP
func (network *Network) serializeMessage(msg Message) ([]byte, error) {
	return json.Marshal(msg)
}

// Uses JSON decoding to parse network data into structured format
func (network *Network) deserializeMessage(data []byte) (Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return msg, err
}

// Continuously listens for incoming UDP messages, handles shutdowns and processes eaach message
func (network *Network) listenLoop() {
	buffer := make([]byte, 8192)
	for {
		// Sets short deadline so we can check for shutdown
		_ = network.conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		n, addr, err := network.conn.ReadFromUDP(buffer)
		if err != nil {
			// deadline: check if we're shutting down
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-network.done:
					return
				default:
					continue
				}
			}
			// socket closed: exit quietly
			if errors.Is(err, net.ErrClosed) ||
				strings.Contains(strings.ToLower(err.Error()), "use of closed network connection") {
				return
			}
			// other transient errors are still logged once
			log.Printf("Error reading from UDP: %v\n", err)
			continue
		}

		// copy message data to avoid corruption
		pkt := make([]byte, n)
		copy(pkt, buffer[:n])
		// goroutine to process each message concurrently
		go network.handleMessage(pkt, addr)
	}
}

// Routes incoming messages to appropriate handlers based on type
func (network *Network) handleMessage(data []byte, addr *net.UDPAddr) {
	// Deserializes raw bytes into Message struct
	msg, err := network.deserializeMessage(data)
	if err != nil {
		log.Printf("Error deserializing message: %v\n", err)
		return
	}

	// Updates routing table with senders contact info
	network.RoutingTable.AddContact(msg.Sender)

	// Calls correct handler based on message type
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

// Processes incoming PING requests and sends PONG responses as confirmation. "Are you there" check
func (network *Network) handlePing(msg Message, addr *net.UDPAddr) {
	log.Printf("Received PING from %s (RPC ID: %s)\n", msg.Sender.Address, msg.RPCID.String())

	// Create PONG response with same RPC ID
	response := Message{
		RPCID:  msg.RPCID, // Use same RPC ID as request
		Type:   PONG,
		Sender: network.RoutingTable.me,
	}

	// Serializes the response
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

// Processes PONG responses and updates routing table
func (network *Network) handlePong(msg Message, addr *net.UDPAddr) {
	log.Printf("Received PONG from %s (RPC ID: %s)\n", msg.Sender.Address, msg.RPCID.String())

	// Always update routing table with sender's contact info (main benefit)
	network.RoutingTable.AddContact(msg.Sender)
	// Optional clean up ping requests
	network.pendingMutex.Lock()
	if _, exists := network.pendingRequests[msg.RPCID.String()]; exists {
		delete(network.pendingRequests, msg.RPCID.String())
		log.Printf("Cleaned up pending PING request for RPC ID %s\n", msg.RPCID.String())
	}
	network.pendingMutex.Unlock()
}

/*
*
*
*
*
*
*
*
*
*
*
 */

// handleFindNode processes FIND_NODE requests, finds closest nodes and sends them back in response
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

// Processes FIND_NODE responses, matches responses to requests
func (network *Network) handleFindNodeResponse(msg Message, addr *net.UDPAddr) {
	log.Printf("Received FIND_NODE_RESPONSE with %d nodes from %s (RPC ID: %s)\n", len(msg.Nodes), msg.Sender.Address, msg.RPCID.String())

	// Check if this is a response to a pending request
	network.pendingMutex.Lock()
	if pendingReq, exists := network.pendingRequests[msg.RPCID.String()]; exists {
		if pendingReq.resultChan != nil {
			select {
			case pendingReq.resultChan <- msg.Nodes:
			default:
			}
			close(pendingReq.resultChan)
		}
		// Deliver to sync response channel if used
		if pendingReq.ResponseChan != nil {
			select {
			case pendingReq.ResponseChan <- msg:
			default:
			}
			close(pendingReq.ResponseChan)
		}

		delete(network.pendingRequests, msg.RPCID.String())
		network.pendingMutex.Unlock()
		log.Printf("Delivered response for RPC ID %s\n", msg.RPCID.String())
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

// SendPingMessage sends a PING message without waiting for response (fire-and-forget)
func (network *Network) SendPingMessage(contact *Contact) error {
	// Generate 160-bit RPC ID
	rpcID := NewRandomKademliaID()

	// Create PING message with RPC ID
	msg := Message{
		RPCID:  rpcID, //160-bit RPC identifier
		Type:   PING,
		Sender: network.RoutingTable.me,
	}

	// Serialize message
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
	return nil // Returns immediately - fire-and-forget
}

/*
*
*
*
*
*
*
*
*
*
*
 */

// Sends a FIND_NODE message to another node to find contacts close to target ID
func (network *Network) SendFindContactMessage(contact *Contact, targetID *KademliaID) ([]Contact, error) {
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

// Does the same thing as the SendFindContacMessage but does it asynchronously on multiple nodes without waitng for them to finish.
func (network *Network) SendFindContactMessageAsync(contact *Contact, targetID *KademliaID) (<-chan []Contact, error) {
	rpcID := NewRandomKademliaID()
	resultChan := make(chan []Contact, 1)

	//Store pending request
	network.pendingMutex.Lock()
	network.pendingRequests[rpcID.String()] = &PendingRequest{
		Timestamp:    time.Now(),
		ResponseChan: make(chan Message, 1),
		TargetID:     targetID,
		resultChan:   resultChan,
	}
	network.pendingMutex.Unlock()

	//Create FIND_NODE message
	msg := Message{
		RPCID:  rpcID,
		Type:   FIND_NODE,
		Sender: network.RoutingTable.me,
		Target: targetID,
	}

	//Serialize and send message
	msgData, err := network.serializeMessage(msg)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		close(resultChan)
		return nil, err
	}

	udpAddr, err := net.ResolveUDPAddr("udp", contact.Address)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		close(resultChan)
		return nil, err
	}

	_, err = network.conn.WriteToUDP(msgData, udpAddr)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		close(resultChan)
		return nil, err
	}
	log.Printf("Sent FIND_NODE to %s for target %s (RPC ID: %s)\n", contact.Address, targetID.String(), rpcID.String())

	//Start timeout goroutine
	go network.startAsyncTimeout(rpcID.String(), resultChan)

	return resultChan, nil

}

// Handles timeout for async requests, if timed out it cleans up the pending request
func (network *Network) startAsyncTimeout(rpcID string, resultChan chan []Contact) {
	time.Sleep(5 * time.Second)

	network.pendingMutex.Lock()
	defer network.pendingMutex.Unlock()

	//Check if request is pending
	if _, exists := network.pendingRequests[rpcID]; exists {
		delete(network.pendingRequests, rpcID)
		//Send empty result to indicate timeout
		select {
		case resultChan <- nil:
		default:
		}
		close(resultChan)
	}
}

/*
*
*
*
*
*
*
*
*
*
*
 */

// Retrieves data form the Kademlia network by its hash
func (network *Network) SendFindDataMessage(hash string) ([]byte, error) {
	// 1. Convert hash to KademliaID
	keyID := NewKademliaID(hash)

	if keyID == nil {
		return nil, errors.New("invalid hash format")
	}
	log.Printf("Looking up data with hash: %s\n", hash)

	// 2. Check if we have the data locally
	network.dataMutex.RLock()
	if data, exists := network.dataStore[hash]; exists {
		network.dataMutex.RUnlock()
		log.Printf("Found data locally for hash %s\n", hash)
		return data, nil
	}
	network.dataMutex.RUnlock()

	// 3. Use iterative lookup to find the data in the network, simple version
	closestNodes := network.RoutingTable.FindClosestContacts(keyID, 3) // Ask α=3 nodes
	if len(closestNodes) == 0 {
		return nil, errors.New("no nodes available for lookup")
	}

	// 4. Send FIND_VALUE requests to closest nodes
	for _, contact := range closestNodes {
		data, err := network.sendFindValueToNode(keyID, &contact)
		if err != nil {
			log.Printf("Error querying node %s: %v\n", contact.Address, err)
			continue
		}
		if data != nil {
			log.Printf("Found data for hash %s on node %s\n", hash, contact.Address)
			return data, nil
		}
	}
	return nil, errors.New("data not found in the network")
}

// Sends a FIND_VALUE message to a specific node to try and retrieve data associated with a given key (KademliaID)
func (network *Network) sendFindValueToNode(keyID *KademliaID, target *Contact) ([]byte, error) {
	// Generate 160-bit RPC ID
	rpcID := NewRandomKademliaID()

	// Create response channel
	respChan := make(chan Message, 1)

	// Store pending request
	network.pendingMutex.Lock()
	network.pendingRequests[rpcID.String()] = &PendingRequest{
		Timestamp:    time.Now(),
		ResponseChan: respChan,
		TargetID:     keyID,
	}
	network.pendingMutex.Unlock()

	// Create FIND_VALUE message
	msg := Message{
		RPCID:  rpcID,
		Type:   FIND_VALUE,
		Sender: network.RoutingTable.me,
		Target: keyID, // Key to find
	}

	// Serialize message
	msgData, err := network.serializeMessage(msg)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, err
	}

	// Resolve target address
	udpAddr, err := net.ResolveUDPAddr("udp", target.Address)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, err
	}

	// Send FIND_VALUE message
	_, err = network.conn.WriteToUDP(msgData, udpAddr)
	if err != nil {
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, err
	}
	log.Printf("Sent FIND_VALUE to %s for key %s\n", target.Address, keyID.String())

	// Wait for response with timeout
	select {
	case response := <-respChan:
		if response.Data != nil {
			return response.Data, nil // Found the data
		}
		return nil, errors.New("node does not have the requesteddata")
	case <-time.After(5 * time.Second):
		network.pendingMutex.Lock()
		delete(network.pendingRequests, rpcID.String())
		network.pendingMutex.Unlock()
		return nil, errors.New("FIND_VALUE request timeout")
	}
}

// Processes incoming FIND_VALUE requests from other nodes
func (network *Network) handleFindValue(msg Message, addr *net.UDPAddr) {
	if msg.Target == nil {
		log.Printf("Invalid FIND_VALUE message: missing target")
		return
	}
	log.Printf("Received FIND_VALUE request for key %s from %s\n", msg.Target.String(), msg.Sender.Address)

	// Check if we have data locally
	network.dataMutex.RLock()
	data, exists := network.dataStore[msg.Target.String()]
	network.dataMutex.RUnlock()

	var response Message
	if exists {
		// We have the data, send it back
		response = Message{
			RPCID:  msg.RPCID,
			Type:   FIND_VALUE_RESPONSE,
			Sender: network.RoutingTable.me,
			Data:   data, // the actual data
		}
		log.Printf("Returning data for key %s (%d bytes)\n", msg.Target.String(), len(data))
	} else {
		// No data, return closest nodes
		closestNodes := network.RoutingTable.FindClosestContacts(msg.Target, 20) // k=20
		response = Message{
			RPCID:  msg.RPCID,
			Type:   FIND_VALUE_RESPONSE,
			Sender: network.RoutingTable.me,
			Nodes:  closestNodes,
		}
		log.Printf("Returning %d closest nodes for key %s\n", len(closestNodes), msg.Target.String())
	}

	// Send response
	responseData, err := network.serializeMessage(response)
	if err != nil {
		log.Printf("Error serializing FIND_VALUE_RESPONSE: %v\n", err)
		return
	}

	_, err = network.conn.WriteToUDP(responseData, addr)
	if err != nil {
		log.Printf("Error sending FIND_VALUE_RESPONSE: %v\n", err)
	}

	// Update routing table with senders info
	network.RoutingTable.AddContact(msg.Sender)
}

// Handles FIND_VALUE response messages which are replies to FIND_VALUE
func (network *Network) handleFindValueResponse(msg Message, addr *net.UDPAddr) {
	log.Printf("Received FIND_VALUE_RESPONSE from %s (RPC ID: %s)\n", msg.Sender.Address, msg.RPCID.String())

	// Check if this is a response to a pending request
	network.pendingMutex.Lock()
	if pendingReq, exists := network.pendingRequests[msg.RPCID.String()]; exists {
		// Send response to waiting goroutine
		pendingReq.ResponseChan <- msg
		delete(network.pendingRequests, msg.RPCID.String())
		network.pendingMutex.Unlock()
		log.Printf("Delivered FIND_VALUE response for RPC ID %s\n", msg.RPCID.String())
	} else {
		network.pendingMutex.Unlock()
		log.Printf("Received unsolicited FIND_VALUE_RESPONSE from %s\n", msg.Sender.Address)
	}

	//Always update senders contact info
	network.RoutingTable.AddContact(msg.Sender)
}

/*
*
*
*
*
*
*
*
*
*
*
 */

// Store data in the Kademlia network och skickar till alla 20 noder
func (network *Network) SendStoreMessage(data []byte) error {
	// 1. Hash data to get storage key 160-bit
	hash := network.hashData(data)
	keyID := NewKademliaID(hash)
	log.Printf("Storing data with hash %s\n", hash)

	// 2. Find k closest nodes to hash
	closestNodes := network.RoutingTable.FindClosestContacts(keyID, 20) // k=20
	if len(closestNodes) == 0 {
		return errors.New("no nodes available for storage")
	}

	// 3. Send STORE messages to all k closest nodes
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(closestNodes))

	for _, contact := range closestNodes {
		wg.Add(1)
		go func(target Contact) {
			defer wg.Done()

			err := network.sendStoreToNode(data, keyID, &target)
			if err != nil {
				errorsChan <- err
			}
		}(contact)
	}

	// Wait for all store operations to complete
	wg.Wait()
	close(errorsChan)

	//Check if any errors occurred
	var storeErrors []error
	for err := range errorsChan {
		storeErrors = append(storeErrors, err)
	}

	if len(storeErrors) > 0 {
		log.Printf("Store completed with %d errors out of %d nodes\n", len(storeErrors), len(closestNodes))
	}

	log.Printf("Successfully stored data on %d nodes\n", len(closestNodes)-len(storeErrors))

	return nil
}

// Sends a STORE message to a specific single node
func (network *Network) sendStoreToNode(data []byte, keyID *KademliaID, target *Contact) error {
	// Generate 160-bit RPC ID
	rpcID := NewRandomKademliaID()

	// Create STORE message
	msg := Message{
		RPCID:  rpcID,
		Type:   STORE,
		Sender: network.RoutingTable.me,
		Target: keyID, //hash key where data is stored
		Data:   data,  //The actual data to store
	}

	// Serialize message
	msgData, err := network.serializeMessage(msg)
	if err != nil {
		return err
	}

	// Resolve target address
	udpAddr, err := net.ResolveUDPAddr("udp", target.Address)
	if err != nil {
		return err
	}

	// Send STORE message via UDP
	_, err = network.conn.WriteToUDP(msgData, udpAddr)
	if err != nil {
		return err
	}

	log.Printf("Sent STORE request to %s for key %s\n", target.Address, keyID.String())
	return nil
}

// hashData creates a SHA-1 hash of the data (160-bit)
func (network *Network) hashData(data []byte) string {
	// Creating a simple placeholder

	hasher := sha1.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))

}

// handles and processes incoming STORE requests from other nodes
func (network *Network) handleStore(msg Message, addr *net.UDPAddr) {
	if msg.Target == nil || msg.Data == nil {
		log.Printf("Invalid STORE message: missing target or data")
		return
	}

	log.Printf("Received STORE request for key %s from %s\n", msg.Target.String(), msg.Sender.Address)

	// Store data locally
	network.dataMutex.Lock()
	network.dataStore[msg.Target.String()] = msg.Data
	network.dataMutex.Unlock()

	// Update routing table with senders info
	network.RoutingTable.AddContact(msg.Sender)
	log.Printf("Stored data for key %s (%d bytes)\n", msg.Target.String(), len(msg.Data))
}

// Returns the nodes own KademliaID
func (n *Network) ID() *KademliaID { return n.nodeID }

// Returns the nodes own network address
func (n *Network) Address() string { return n.address }

// Shuts down the network node
func (n *Network) Close() error {
	select {
	case <-n.done: // already closed
		return nil
	default:
		close(n.done)
		// Closing the UDP conn will unblock ReadFromUDP
		return n.conn.Close()
	}
}
