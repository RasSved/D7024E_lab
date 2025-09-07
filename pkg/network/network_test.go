package network

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// pickFreePort returns an available TCP port on 127.0.0.1.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type receipt struct {
	from int
	to   int
	typ  string
}

func parseType(payload []byte) (string, string) {
	s := string(payload)
	if i := strings.IndexByte(s, ':'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return "default", s
}

func TestTCPAnyToAnyMessaging(t *testing.T) {
	// If your tcp_network.go exposes NewTCPNetwork(), use it.
	// Otherwise, ensure a constructor exists that returns a value implementing Network.
	netw := NewTCPNetwork()

	const N = 10 // keep tests quick; bump if you like
	types := []string{"hello", "json", "bytes", "emoji", "long"}

	// Build N nodes with fixed ports so msg.From contains a non-zero port.
	nodes := make([]*Node, N)
	addrs := make([]Address, N)
	addrToIdx := make(map[string]int)

	for i := 0; i < N; i++ {
		port := pickFreePort(t)
		addrs[i] = Address{IP: "127.0.0.1", Port: port}

		node, err := NewNode(netw, addrs[i])
		if err != nil {
			t.Fatalf("node %d listen: %v", i, err)
		}
		nodes[i] = node

		key := fmt.Sprintf("%s:%d", addrs[i].IP, addrs[i].Port)
		addrToIdx[key] = i
	}

	// Channel to collect receipts
	want := N * len(types) // one message per (node, type)
	recvCh := make(chan receipt, want)

	// Register a single default handler that records messages.
	for i, n := range nodes {
		i := i // capture
		n.Handle("default", func(msg Message) error {
			typ, _ := parseType(msg.Payload)
			fromKey := fmt.Sprintf("%s:%d", msg.From.IP, msg.From.Port)
			fromIdx, ok := addrToIdx[fromKey]
			if !ok {
				// We still record, but this indicates From had unexpected addr (e.g., port 0).
				fromIdx = -1
			}
			recvCh <- receipt{from: fromIdx, to: i, typ: typ}
			return nil
		})
		n.Start()
	}

	// Give listeners a brief moment to enter Recv loops.
	time.Sleep(150 * time.Millisecond)

	// Send one message of each type from every node to a different node (next in ring).
	var sendWG sync.WaitGroup
	sendWG.Add(want)

	for i, n := range nodes {
		to := addrs[(i+1)%N]
		for _, typ := range types {
			i, typ := i, typ
			n, to := n, to
			go func() {
				defer sendWG.Done()
				switch typ {
				case "hello":
					_ = n.SendString(to, "hello", fmt.Sprintf("hello from %d to %d", i, (i+1)%N))
				case "json":
					body, _ := json.Marshal(map[string]any{
						"from": i, "to": (i + 1) % N, "ok": true,
					})
					_ = n.Send(to, "json", body)
				case "bytes":
					_ = n.Send(to, "bytes", []byte{0, 1, 2, 3, 4, 5})
				case "emoji":
					_ = n.SendString(to, "emoji", fmt.Sprintf("👋 from %d → %d", i, (i+1)%N))
				case "long":
					payload := make([]byte, 4096)
					for i := range payload {
						payload[i] = 'x'
					}
					_ = n.Send(to, "long", payload)
				default:
					_ = n.SendString(to, typ, "data")
				}
			}()
		}
	}
	sendWG.Wait()

	// Build the expected set
	type key struct {
		from, to int
		typ      string
	}
	expected := make(map[key]struct{}, want)
	for i := 0; i < N; i++ {
		toIdx := (i + 1) % N
		for _, typ := range types {
			expected[key{i, toIdx, typ}] = struct{}{}
		}
	}

	// Collect until all expected found or timeout
	deadline := time.After(5 * time.Second)
	for len(expected) > 0 {
		select {
		case r := <-recvCh:
			k := key(r)
			delete(expected, k)
		case <-deadline:
			// Build a helpful error report
			var missing []string
			for k := range expected {
				missing = append(missing, fmt.Sprintf("%s %d→%d", k.typ, k.from, k.to))
			}
			t.Fatalf("timed out waiting for %d deliveries; missing:\n  %s",
				len(expected), strings.Join(missing, "\n  "))
		}
	}

	// Cleanup
	for _, n := range nodes {
		_ = n.Close()
	}
}
