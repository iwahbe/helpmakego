package cmd

import (
	"github.com/spf13/cobra"

	pkg_deamon "github.com/iwahbe/helpmakego/internal/pkg/deamon"
)

func deamon() *cobra.Command {
	cmd := &cobra.Command{
		Use: "deamon moduleRoot",
		// This cmd is used internally, but doesn't need to be exposed to users.
		DisableSuggestions: true,
		Hidden:             true,
		Args:               cobra.ExactArgs(1),
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return pkg_deamon.Serve(cmd.Context(), args[0])
	}

	return cmd
}
