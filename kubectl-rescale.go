package main

import (
	"os"

	"github.com/spf13/pflag"
	"github.com/wywywywy/kubectl-rescale/pkg/cmd"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func main() {
	flags := pflag.NewFlagSet("kubectl-rescale", pflag.ExitOnError)
	pflag.CommandLine = flags

	rootCmd := cmd.NewCmdRescale(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
