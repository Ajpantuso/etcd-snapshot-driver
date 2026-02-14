package main

import (
	"fmt"

	"github.com/Ajpantuso/etcd-snapshot-driver/internal/config"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print the version, build time, and git commit of etcd-snapshot-driver",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("etcd-snapshot-driver version %s (built %s, commit %s)\n",
				config.Version,
				config.BuildTime,
				config.GitCommit,
			)
		},
	}
}
