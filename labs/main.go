package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func getenv(k, d string) string { // Reads env variable ex port info
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func whoami() string { // Sets name for node
	if n := os.Getenv("NODE_NAME"); n != "" {
		return n
	}
	h, _ := os.Hostname()
	if h == "" {
		return "unknown"
	}
	return h
}

func parseKV(s string) map[string]string { // Parses payload
	m := map[string]string{}
	for _, p := range strings.Split(s, "|") {
		kv := strings.SplitN(p, ":", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = kv[1]
		}
	}
	return m
}

func sendUDP(toHost, port, payload string) error {
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%s", toHost, port)) // Find adress for node
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, raddr) // Open UDP socket ("connection")
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(payload)) // Send payload
	return err
}

func runServer(node, port string) error {
	laddr, err := net.ResolveUDPAddr("udp", ":"+port) // Bind UDP socket
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", laddr) // Listen on the socket
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("%s listening on UDP :%s", node, port)

	buf := make([]byte, 65535) // Max payload size
	for {
		n, addr, err := conn.ReadFromUDP(buf) // Gets a payload, takes it as string use parse to get info
		if err != nil {
			continue
		}
		raw := string(buf[:n])
		fields := parseKV(raw)
		typ := fields["type"]
		id := fields["id"]
		from := fields["from"]
		body := fields["body"]

		if typ == "ack" { // If ack just log it
			log.Printf("%s ACK id=%s from=%s (%s)", node, id, from, addr.String())
			continue
		}

		log.Printf("%s got id=%s from=%s (%s): %q", node, id, from, addr.String(), body) // Log recieve

		if getenv("AUTO_ACK", "1") == "1" && from != "" { // Send ack back to sender
			ack := fmt.Sprintf("type:ack|id:%s|from:%s|body:ok", id, node)
			_ = sendUDP(from, port, ack)
		}
	}
}

func main() {
	node := whoami()
	port := getenv("PORT", "9999")

	if len(os.Args) > 1 && os.Args[1] == "send" { // Checks if we run as sender
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: %s send <host> <message>\n", os.Args[0]) // If all the needed info sint there
			os.Exit(2)
		}
		host := os.Args[2] // Declare sender
		msg := strings.Join(os.Args[3:], " ")
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		payload := fmt.Sprintf("type:msg|id:%s|from:%s|body:%s", id, node, msg)

		if err := sendUDP(host, port, payload); err != nil { // Check fail then log
			log.Fatalf("send error: %v", err)
		}
		log.Printf("%s -> %s id=%s (UDP)", node, host, id)
		return
	}

	if err := runServer(node, port); err != nil { // Default to run as server (reciever)
		log.Fatal(err)
	}
}
