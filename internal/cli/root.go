package cli

import (
	"fmt" //output
	"os"
	"os/signal"
	"syscall"

	"github.com/RasSved/D7024E_lab/internal/kademlia" //DHT logic
	"github.com/spf13/cobra" //framework for building CLI commands
)

var (
	flagIP        string
	flagPort      int
	flagBootstrap string
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "kademlia",
		Short: "Kademlia DHT CLI",
	}
	// Global flags for all subcommands
	root.PersistentFlags().StringVar(&flagIP, "ip", "127.0.0.1", "listen IP")
	root.PersistentFlags().IntVar(&flagPort, "port", 4001, "UDP port to listen on")
	root.PersistentFlags().StringVar(&flagBootstrap, "bootstrap", "", "bootstrap host:port")

	// Subcommands
	root.AddCommand(
		newServeCmd(), //creates the root cobra command kademlia
		newPutCmd(), //Stores a string in the DHT and creates a hash key for lookup
		newGetCmd(), //retrives a value from the hash key
		newVersionCmd(), //shows version
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
				fmt.Printf("Object found locally: %s\n", string(data))
			} else {
				fmt.Printf("Got from %s: %s\n", from.Address, string(data))
			}
			return nil
		},
	}
}

//func newExitCmd(node *kademlia.Kademlia) *cobra.Command {
//	return &cobra.Command{
//		Use: "exit",
//		Run: func(cmd *cobra.Command, args []string) {
//			_ = node.Close()
//			fmt.Println("Shutting down…")
//			os.Exit(0)
//		},
//	}
//}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the node and keep it running until Ctrl+C",
		RunE: func(cmd *cobra.Command, args []string) error {
			node, err := kademlia.NewNode(flagIP, flagPort, flagBootstrap)
			if err != nil {
				return err
			}
			defer node.Close()

			fmt.Println("Node running at", node.Address())

			// Block until killed
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
			<-sig
			return node.Close()
		},
	}
}
