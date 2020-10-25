package cmd

import (
	"github.com/spf13/cobra"

	"github.com/twpayne/chezmoi/next/internal/chezmoi"
)

type editCmdConfig struct {
	apply     bool
	diff      bool
	include   *chezmoi.IncludeSet
	prompt    bool
	recursive bool
}

func (c *Config) newEditCmd() *cobra.Command {
	editCmd := &cobra.Command{
		Use:     "edit targets...",
		Short:   "Edit the source state of a target",
		Long:    mustGetLongHelp("edit"),
		Example: getExample("edit"),
		RunE:    c.makeRunEWithSourceState(c.runEditCmd),
		Annotations: map[string]string{
			modifiesDestinationDirectory: "true", // Only if --apply. FIXME use exec instead.
			modifiesSourceDirectory:      "true",
			requiresSourceDirectory:      "true",
			runsCommands:                 "true",
		},
	}

	persistentFlags := editCmd.PersistentFlags()
	persistentFlags.BoolVarP(&c.edit.apply, "apply", "a", c.edit.apply, "apply edit after editing")
	persistentFlags.BoolVarP(&c.edit.diff, "diff", "d", c.edit.diff, "print diff after editing")
	persistentFlags.BoolVarP(&c.edit.prompt, "prompt", "p", c.edit.prompt, "prompt before applying (implies --diff)")

	markRemainingZshCompPositionalArgumentsAsFiles(editCmd, 1)

	return editCmd
}

func (c *Config) runEditCmd(cmd *cobra.Command, args []string, s *chezmoi.SourceState) error {
	var sourcePaths []string
	if len(args) == 0 {
		sourcePaths = []string{c.absSourceDir}
	} else {
		var err error
		sourcePaths, err = c.getSourcePaths(s, args)
		if err != nil {
			return err
		}
	}

	// FIXME transparently decrypt encrypted files

	if err := c.runEditor(sourcePaths); err != nil {
		return err
	}

	if !c.edit.apply {
		return nil
	}

	// FIXME --diff
	// FIXME --prompt
	return c.applyArgs(c.destSystem, c.absDestDir, args, c.edit.include, c.edit.recursive, c.Umask.FileMode())
}
