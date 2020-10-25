package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func (c *Config) newHelpCmd(rootCmd *cobra.Command) *cobra.Command {
	helpCmd := &cobra.Command{
		Use:     "help [command]",
		Short:   "Print help about a command",
		Long:    mustGetLongHelp("help"),
		Example: getExample("help"),
		RunE: func(cmd *cobra.Command, args []string) error {
			subCmd, _, err := rootCmd.Find(args)
			if err != nil {
				return err
			}
			if subCmd == nil {
				return fmt.Errorf("unknown command: %q", strings.Join(args, " "))
			}
			return subCmd.Help()
		},
	}
	return helpCmd
}
