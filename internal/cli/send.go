package cli

import (
	"fmt"
	"net"
	"strconv"

	"github.com/RasSved/D7024E_lab/pkg/network"
	"github.com/spf13/cobra"
)

var toFlag, typeFlag, dataFlag string

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a single message to IP:port",
	RunE: func(cmd *cobra.Command, args []string) error {
		host, pStr, err := net.SplitHostPort(toFlag)
		if err != nil {
			return fmt.Errorf("bad --to: %w", err)
		}
		port, _ := strconv.Atoi(pStr)

		nw := network.NewTCPNetwork()
		conn, err := nw.Dial(network.Address{IP: host, Port: port})
		if err != nil {
			return err
		}
		defer conn.Close()

		return conn.Send(network.Message{
			From:    network.Address{IP: detectIPv4(), Port: 0},
			To:      network.Address{IP: host, Port: port},
			Payload: []byte(typeFlag + ":" + dataFlag),
			// network is not required for one-shot send
		})
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)
	sendCmd.Flags().StringVar(&toFlag, "to", "", "target IP:port (required)")
	sendCmd.Flags().StringVar(&typeFlag, "type", "msg", "message type")
	sendCmd.Flags().StringVar(&dataFlag, "data", "", "payload")
	_ = sendCmd.MarkFlagRequired("to")
}
