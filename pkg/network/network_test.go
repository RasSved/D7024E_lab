package network

import (
	"fmt"
	"testing"
)

func TestHelloworld(t *testing.T) {
	// Create network and nodes
	network := NewMockNetwork()
	alice, _ := NewNode(network, Address{IP: "127.0.0.1", Port: 8080})
	bob, _ := NewNode(network, Address{IP: "127.0.0.1", Port: 8081})

	// Channel for synchronization
	done := make(chan struct{})

	// Alice says hello when she receives a message
	alice.Handle("hello", func(msg Message) error {
		fmt.Printf("Alice: Hello %s!\n", msg.From.IP)
		return msg.ReplyString("reply", "Nice to meet you!")
	})

	// Bob prints replies
	bob.Handle("reply", func(msg Message) error {
		fmt.Printf("Bob: %s\n", string(msg.Payload)[6:]) // Skip "reply:" prefix
		done <- struct{}{}
		return nil
	})

	// Start nodes and send message
	alice.Start()
	bob.Start()
	bob.SendString(alice.Address(), "hello", "Hi Alice!")

	// Wait for completion and cleanup
	<-done
	alice.Close()
	bob.Close()
	fmt.Println("Done!")
}
