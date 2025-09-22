package cli

import (
	"fmt"

	"github.com/RasSved/D7024E_lab/pkg/build"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Version: %s\n", build.BuildVersion) //what version
			fmt.Printf("Built at: %s\n", build.BuildTime) //when it was built
		},
	}
}
