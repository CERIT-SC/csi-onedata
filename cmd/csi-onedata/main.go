package main

import (
	"flag"
	"fmt"
	"csi-onedata/pkg/oneclient"
	"os"

	"github.com/spf13/cobra"
)

var (
	endpoint string
	nodeID   string
)

func init() {
	flag.Set("logtostderr", "true")
}

func main() {

	flag.CommandLine.Parse([]string{})

	cmd := &cobra.Command{
		Use:   "oneclient",
		Short: "CSI based OneData driver",
		Run: func(cmd *cobra.Command, args []string) {
			handle()
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	cmd.PersistentFlags().StringVar(&nodeID, "nodeid", "", "node id")
	cmd.MarkPersistentFlagRequired("nodeid")

	cmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "CSI endpoint")
	cmd.MarkPersistentFlagRequired("endpoint")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Prints information about this version of csi onedata plugin",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(`CSI-OneData Plugin
Version:    %s
Build Time: %s
`, oneclient.Version, oneclient.BuildTime)
		},
	}

	cmd.AddCommand(versionCmd)
	versionCmd.ResetFlags()

	cmd.ParseFlags(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s", err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}

func handle() {
	d := oneclient.NewDriver(nodeID, endpoint)
	d.Run()
}
