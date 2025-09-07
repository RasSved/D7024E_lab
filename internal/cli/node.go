// internal/cli/node.go
package cli

import (
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/RasSved/D7024E_lab/pkg/network"
)

var flagListen string // ":4000" or "0.0.0.0:4000"

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Run a TCP node",
	RunE: func(cmd *cobra.Command, args []string) error {
		host, port := splitHostPort(flagListen)
		if host == "" || host == "0.0.0.0" {
			host = detectIPv4()
		}
		self := network.Address{IP: host, Port: port}

		netw, err := func() (network.Network, error) { return network.NewTCPNetwork(), nil }()
		if err != nil {
			return err
		}

		n, err := network.NewNode(netw, self)
		if err != nil {
			return err
		}

		n.Handle("default", func(m network.Message) error {
			log.Printf("[%s] recv from %s: %q", self.String(), m.From.String(), string(m.Payload))
			return nil
		})

		n.Start()
		log.Printf("node listening on 0.0.0.0:%d (advertise %s)", self.Port, self.String())

		// Block until SIGINT/SIGTERM
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		return n.Close()
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
	nodeCmd.Flags().StringVar(&flagListen, "listen", ":4000", "listen address (host:port). host is used only for advertising")
}

func splitHostPort(s string) (string, int) {
	h, pStr, err := net.SplitHostPort(s)
	if err != nil && strings.HasPrefix(s, ":") {
		h = "0.0.0.0"
		pStr = strings.TrimPrefix(s, ":")
	}
	p, _ := strconv.Atoi(pStr)
	return h, p
}

func detectIPv4() string {
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}
