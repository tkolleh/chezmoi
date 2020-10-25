package cmd

import (
	"github.com/spf13/cobra"
	"github.com/twpayne/go-shell"
)

type cdCmdConfig struct {
	Command string
	Args    []string
}

func (c *Config) newCDCmd() *cobra.Command {
	cdCmd := &cobra.Command{
		Use:     "cd",
		Short:   "Launch a shell in the source directory",
		Long:    mustGetLongHelp("cd"),
		Example: getExample("cd"),
		RunE:    c.runCDCmd,
		Args:    cobra.NoArgs,
		Annotations: map[string]string{
			doesNotRequireValidConfig: "true",
			requiresSourceDirectory:   "true",
			runsCommands:              "true",
		},
	}
	return cdCmd
}

func (c *Config) runCDCmd(cmd *cobra.Command, args []string) error {
	shellCommand := c.CD.Command
	if shellCommand == "" {
		shellCommand, _ = shell.CurrentUserShell()
	}
	return c.run(c.absSourceDir, shellCommand, c.CD.Args)
}
