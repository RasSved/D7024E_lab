package main

import (
	"github.com/RasSved/D7024E_lab/internal/cli"
	"github.com/RasSved/D7024E_lab/pkg/build"
)

var (
	BuildVersion string = ""
	BuildTime    string = ""
)

func main() {
	build.BuildVersion = BuildVersion
	build.BuildTime = BuildTime
	cli.Execute()
}
