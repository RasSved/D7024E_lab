package cli

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/RasSved/D7024E_lab/internal/kademlia" //DHT logic
	"github.com/spf13/cobra"                          //framework for building CLI commands
)

var (
	flagIP        string
	flagPort      int
	flagBootstrap string
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kademlia",
		Short: "Kademlia CLI",
	}
	// Global flags for all subcommands
	root.PersistentFlags().StringVar(&flagIP, "ip", "127.0.0.1", "listen IP")
	root.PersistentFlags().IntVar(&flagPort, "port", 4001, "UDP port to listen on")
	root.PersistentFlags().StringVar(&flagBootstrap, "bootstrap", "", "bootstrap host:port")

	// Subcommands
	root.AddCommand(
		newServeCmd(),
		newPutCmd(),
		newGetCmd(),
		newExitCmd(),
		newVersionCmd(),
	)
	return root
}

func newPutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "put [string]",
		Short: "Store a string in the DHT",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := kademlia.NewNode(flagIP, flagPort, flagBootstrap)
			if err != nil {
				return err
			}
			defer node.Close()

			hash, err := node.Put([]byte(args[0]))
			if err != nil {
				return err
			}
			fmt.Println("Stored with hash:", hash)
			return nil
		},
	}
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get [hash]",
		Short: "Retrieve an object by its hash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := kademlia.NewNode(flagIP, flagPort, flagBootstrap)
			if err != nil {
				return err
			}
			defer node.Close()

			data, from, err := node.Get(args[0])
			if err != nil {
				return err
			}
			if from == nil {
				fmt.Printf("Object found: %s\n", string(data))
			} else {
				fmt.Printf("Got from %s: %s\n", from.Address, string(data))
			}
			return nil
		},
	}
}

func newExitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exit",
		Short: "Terminate a running node started with 'serve'",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Must match serve’s control address derivation
			ctrl := net.JoinHostPort("127.0.0.1", strconv.Itoa(flagPort+10000))
			addr, err := net.ResolveUDPAddr("udp", ctrl)
			if err != nil {
				return fmt.Errorf("resolve: %w", err)
			}
			conn, err := net.DialUDP("udp", nil, addr)
			if err != nil {
				return fmt.Errorf("dial: %w", err)
			}
			defer conn.Close()

			_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write([]byte("EXIT")); err != nil {
				return fmt.Errorf("send: %w", err)
			}
			return nil
		},
	}
}

//func newServeCmd() *cobra.Command {
//	return &cobra.Command{
//		Use:   "serve",
//		Short: "Run the node and keep it running until Ctrl+C",
//		RunE: func(cmd *cobra.Command, args []string) error {
//			node, err := kademlia.NewNode(flagIP, flagPort, flagBootstrap)
//			if err != nil {
//				return err
//			}
//			defer node.Close()
//
//			fmt.Println("Node running at", node.Address())
//
//			// Block until killed
//			sig := make(chan os.Signal, 1)
//			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
//			<-sig
//			return node.Close()
//		},
//	}
//}

func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the node until stopped",
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := kademlia.NewNode(flagIP, flagPort, flagBootstrap)
			if err != nil {
				return err
			}
			log.Printf("Node running at %s", node.Network().Address())

			// Control port on localhost: port+10000 (avoids privilege and conflicts)
			ctrlAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(flagPort+10000))
			ctrlUDPAddr, err := net.ResolveUDPAddr("udp", ctrlAddr)
			if err != nil {
				_ = node.Close()
				return fmt.Errorf("resolve control addr: %w", err)
			}
			ctrlConn, err := net.ListenUDP("udp", ctrlUDPAddr)
			if err != nil {
				_ = node.Close()
				return fmt.Errorf("listen control: %w", err)
			}
			defer ctrlConn.Close()

			// Quit channels
			stop := make(chan struct{})
			done := make(chan struct{})

			// 1) Control loop for EXIT messages
			go func() {
				defer close(done)
				buf := make([]byte, 16)
				for {
					_ = ctrlConn.SetReadDeadline(time.Now().Add(2 * time.Second))
					n, addr, err := ctrlConn.ReadFromUDP(buf)
					select {
					case <-stop:
						return
					default:
					}
					if err != nil {
						// timeout -> just loop
						if ne, ok := err.(net.Error); ok && ne.Timeout() {
							continue
						}
						log.Printf("control read error: %v", err)
						continue
					}
					msg := string(buf[:n])
					if msg == "EXIT" {
						log.Printf("control: EXIT from %s", addr)
						close(stop)
						return
					}
				}
			}()

			// 2) OS signal handler (Ctrl+C / docker stop)
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-sigs:
				log.Printf("signal: shutting down")
			case <-stop:
				log.Printf("control: shutting down")
			}

			if err := node.Close(); err != nil {
				return err
			}
			<-done // wait control goroutine cleanly exit
			return nil
		},
	}
	return cmd
}
