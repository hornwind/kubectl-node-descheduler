package main

import (
	"os"

	"github.com/hornwind/kubectl-node-descheduler/pkg/cmd"
	"github.com/spf13/pflag"
)

func main() {
	flags := pflag.NewFlagSet("node-descheduler", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewCmdDescheduler()
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
