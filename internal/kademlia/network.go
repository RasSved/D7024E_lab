package kademlia

import (
	"log"
	"net"
	"strconv"
	"sync"
)

// Network represents a Kademlia network node with IDP communication capabilities
type Network struct {
	nodeID       *KademliaID
	address      string
	conn         *net.UDPConn
	RoutingTable *RoutingTable
	dataStore    map[string][]byte
	dataMutex    sync.RWMutex
	rpcMutex     sync.Mutex
	rpcCallbacks map[string]chan RPCResponse // For handling async responses
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
		nodeID:       nodeID,
		address:      addrStr,
		conn:         conn,
		RoutingTable: NewRoutingTable(myContact),
		dataStore:    make(map[string][]byte),
		rpcCallbacks: make(map[string]chan RPCResponse),
	}

	// Start background listener for incoming messages
	go network.listenLoop()

	return network
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
	// Simple text based protocol for PING(temporary implementation)
	if string(data) == "PING" {
		network.handlePing(addr)
		return
	}

	//TODO add parsing for other message types
	log.Printf("Received unknown message: %s\n", string(data))
}

// handlePing processes PING requests and sends PONG responses
func (network *Network) handlePing(addr *net.UDPAddr) {
	response := []byte("PONG")
	_, err := network.conn.WriteToUDP(response, addr)
	if err != nil {
		log.Printf("Error sending PONG response: %v\n", err)
	}
}

// SendPingMessage sends a PING message to the specified contact
func (network *Network) SendPingMessage(contact *Contact) error {
	// Resolve the target address
	udpAddr, err := net.ResolveUDPAddr("udp", contact.Address)
	if err != nil {
		return err
	}

	// Send PING message via UDP
	_, err = network.conn.WriteToUDP([]byte("PING"), udpAddr)
	return err
}

//
//
//

func (network *Network) SendFindContactMessage(contact *Contact) {
	// TODO
}

func (network *Network) SendFindDataMessage(hash string) {
	// TODO
}

func (network *Network) SendStoreMessage(data []byte) {
	// TODO
}
