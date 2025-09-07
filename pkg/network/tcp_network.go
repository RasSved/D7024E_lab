package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

type tcpNetwork struct {
	mu        sync.RWMutex
	listeners map[Address]*tcpListener
}

type tcpListener struct {
	addr   Address
	ln     net.Listener
	recvCh chan Message
	netref *tcpNetwork
}

type tcpDialConn struct {
	c net.Conn
}

func NewTCPNetwork() Network {
	return &tcpNetwork{listeners: make(map[Address]*tcpListener)}
}

func (n *tcpNetwork) Listen(addr Address) (Connection, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.listeners[addr]; ok {
		return nil, errors.New("address already in use")
	}

	// Bind on all interfaces inside containers
	bind := fmt.Sprintf("0.0.0.0:%d", addr.Port)
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, err
	}

	l := &tcpListener{
		addr:   addr,
		ln:     ln,
		recvCh: make(chan Message, 256),
		netref: n,
	}
	n.listeners[addr] = l
	go l.acceptLoop()
	return l, nil
}

func (n *tcpNetwork) Dial(addr Address) (Connection, error) {
	c, err := net.Dial("tcp", addr.String())
	if err != nil {
		return nil, err
	}
	return &tcpDialConn{c: c}, nil
}

func (n *tcpNetwork) Partition(_, _ []Address) {}
func (n *tcpNetwork) Heal()                    {}

func (l *tcpListener) acceptLoop() {
	for {
		c, err := l.ln.Accept()
		if err != nil {
			close(l.recvCh)
			return
		}
		go l.handleConn(c)
	}
}

func (l *tcpListener) handleConn(c net.Conn) {
	defer c.Close()
	dec := json.NewDecoder(c)
	for {
		var m Message
		if err := dec.Decode(&m); err != nil {
			return
		}
		// Make Reply() work and stamp local To
		m.network = l.netref
		m.To = l.addr
		select {
		case l.recvCh <- m:
		default: // drop if overwhelmed
		}
	}
}

// Connection impls
func (l *tcpListener) Send(Message) error { return errors.New("listener cannot Send") }
func (l *tcpListener) Recv() (Message, error) {
	msg, ok := <-l.recvCh
	if !ok {
		return Message{}, errors.New("closed")
	}
	return msg, nil
}
func (l *tcpListener) Close() error { _ = l.ln.Close(); return nil }

func (d *tcpDialConn) Recv() (Message, error) {
	return Message{}, errors.New("dial connection cannot Recv")
}
func (d *tcpDialConn) Send(msg Message) error { return json.NewEncoder(d.c).Encode(msg) }
func (d *tcpDialConn) Close() error           { return d.c.Close() }
