package main

import (
	"log"

	"github.com/RasSved/D7024E_lab/internal/cli"
)

// Entry point
// Build root command that has our made subcommands
// From subcommands we creat and run kademlia nodes
func main() {
	root := cli.NewRootCmd()
	if err := root.Execute(); err != nil {
		log.Fatal(err) //If command execution fails, it logs the error and exits
	}
}
