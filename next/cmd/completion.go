package cmd

// FIXME update REFERENCE.md
// FIXME add per-shell Long and Example

import (
	"strings"

	"github.com/spf13/cobra"
)

var (
	completionCmd = &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion code",
		// Long:      mustGetLongHelp("completion"),
		// Example:   getExample("completion"),
	}

	bashCompletionCmd = &cobra.Command{
		Use:   "bash",
		Args:  cobra.NoArgs,
		Short: "Generate bash completion code",
		RunE:  config.runBashCompletionCmd,
		Annotations: map[string]string{
			doesNotRequireValidConfig: "true",
		},
	}

	fishCompletionCmd = &cobra.Command{
		Use:   "fish",
		Args:  cobra.NoArgs,
		Short: "Generate fish completion code",
		RunE:  config.runFishCompletionCmd,
		Annotations: map[string]string{
			doesNotRequireValidConfig: "true",
		},
	}

	powerShellCompletionCmd = &cobra.Command{
		Use:   "powershell",
		Args:  cobra.NoArgs,
		Short: "Generate PowerShell completion code",
		RunE:  config.runPowerShellCompletionCmd,
		Annotations: map[string]string{
			doesNotRequireValidConfig: "true",
		},
	}

	zshCompletionCmd = &cobra.Command{
		Use:   "zsh",
		Args:  cobra.NoArgs,
		Short: "Generate zsh completion code",
		RunE:  config.runZshCompletionCmd,
		Annotations: map[string]string{
			doesNotRequireValidConfig: "true",
		},
	}
)

func init() {
	completionCmd.AddCommand(bashCompletionCmd)
	completionCmd.AddCommand(fishCompletionCmd)
	completionCmd.AddCommand(powerShellCompletionCmd)
	completionCmd.AddCommand(zshCompletionCmd)
	rootCmd.AddCommand(completionCmd)
}

func (c *Config) runBashCompletionCmd(cmd *cobra.Command, args []string) error {
	sb := &strings.Builder{}
	if err := rootCmd.GenBashCompletion(sb); err != nil {
		return err
	}
	return c.writeOutputString(sb.String())
}

func (c *Config) runFishCompletionCmd(cmd *cobra.Command, args []string) error {
	sb := &strings.Builder{}
	if err := rootCmd.GenFishCompletion(sb, true); err != nil {
		return err
	}
	return c.writeOutputString(sb.String())
}

func (c *Config) runPowerShellCompletionCmd(cmd *cobra.Command, args []string) error {
	sb := &strings.Builder{}
	if err := rootCmd.GenPowerShellCompletion(sb); err != nil {
		return err
	}
	return c.writeOutputString(sb.String())
}

func (c *Config) runZshCompletionCmd(cmd *cobra.Command, args []string) error {
	sb := &strings.Builder{}
	if err := rootCmd.GenZshCompletion(sb); err != nil {
		return err
	}
	return c.writeOutputString(sb.String())
}
