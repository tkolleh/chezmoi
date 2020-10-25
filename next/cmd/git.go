package cmd

import (
	"github.com/spf13/cobra"
)

type gitCmdConfig struct {
	Command    string
	AutoAdd    bool
	AutoCommit bool
	AutoPush   bool
}

func (c *Config) newGitCmd() *cobra.Command {
	gitCmd := &cobra.Command{
		Use:     "git [args...]",
		Short:   "Run git in the source directory",
		Long:    mustGetLongHelp("git"),
		Example: getExample("git"),
		RunE:    c.runGitCmd,
		Annotations: map[string]string{
			requiresSourceDirectory: "true",
			runsCommands:            "true",
		},
	}
	return gitCmd
}

func (c *Config) runGitCmd(cmd *cobra.Command, args []string) error {
	return c.run(c.absSourceDir, c.Git.Command, args)
}
