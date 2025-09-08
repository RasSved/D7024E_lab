package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func whoami() string {
	if n := os.Getenv("NODE_NAME"); n != "" {
		return n
	}
	h, _ := os.Hostname()
	if h == "" {
		return "unknown"
	}
	return h
}

func parseKV(s string) map[string]string {
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
	raddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%s", toHost, port))
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(payload))
	return err
}

func runServer(node, port string) error {
	laddr, err := net.ResolveUDPAddr("udp", ":"+port)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Printf("%s listening on UDP :%s", node, port)

	buf := make([]byte, 65535)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		raw := string(buf[:n])
		fields := parseKV(raw)
		typ := fields["type"]
		id := fields["id"]
		from := fields["from"]
		body := fields["body"]

		if typ == "ack" {
			log.Printf("%s ACK id=%s from=%s (%s)", node, id, from, addr.String())
			continue
		}

		log.Printf("%s got id=%s from=%s (%s): %q", node, id, from, addr.String(), body)

		if getenv("AUTO_ACK", "1") == "1" && from != "" {
			ack := fmt.Sprintf("type:ack|id:%s|from:%s|body:ok", id, node)
			_ = sendUDP(from, port, ack)
		}
	}
}

func main() {
	node := whoami()
	port := getenv("PORT", "9999")

	if len(os.Args) > 1 && os.Args[1] == "send" {
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "usage: %s send <host> <message>\n", os.Args[0])
			os.Exit(2)
		}
		host := os.Args[2]
		msg := strings.Join(os.Args[3:], " ")
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		payload := fmt.Sprintf("type:msg|id:%s|from:%s|body:%s", id, node, msg)
		if err := sendUDP(host, port, payload); err != nil {
			log.Fatalf("send error: %v", err)
		}
		log.Printf("%s -> %s id=%s (UDP)", node, host, id)
		return
	}

	if err := runServer(node, port); err != nil {
		log.Fatal(err)
	}
}
