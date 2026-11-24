// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func init() {
	cobra.AddTemplateFunc("helpFirstFlagUsages", helpFirstFlagUsages)
}

// Root prepares a binary's root command: errors are left to WriteError and
// help uses the shared layout, which its descendants inherit.
func Root(root *cobra.Command) *cobra.Command {
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.SetUsageTemplate(usageTemplate)
	return root
}

// Group is a command that only holds subcommands and prints help.
func Group(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
}

// Completion generates Bash, Zsh and Fish completion for root.
func Completion(root *cobra.Command) *cobra.Command {
	completion := &cobra.Command{Use: "completion", Short: "Generate shell completion scripts", Args: cobra.NoArgs}
	completion.AddCommand(
		&cobra.Command{
			Use: "bash", Short: "Generate Bash completion script", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return root.GenBashCompletionV2(cmd.OutOrStdout(), true) },
		},
		&cobra.Command{
			Use: "zsh", Short: "Generate Zsh completion script", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return root.GenZshCompletion(cmd.OutOrStdout()) },
		},
		&cobra.Command{
			Use: "fish", Short: "Generate Fish completion script", Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error { return root.GenFishCompletion(cmd.OutOrStdout(), true) },
		},
	)
	return completion
}

// Keep pflag's formatting and alignment while ordering only the display copy.
func helpFirstFlagUsages(flags *pflag.FlagSet) string {
	ordered := pflag.NewFlagSet("help", pflag.ContinueOnError)
	ordered.SortFlags = false
	if help := flags.Lookup("help"); help != nil {
		ordered.AddFlag(help)
	}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Name != "help" {
			ordered.AddFlag(flag)
		}
	})
	return ordered.FlagUsages()
}

// usageTemplate combines flags and subcommands in one usage line and lists
// flag sections before subcommands, retaining Cobra's command usage details.
const usageTemplate = `Usage:{{if or .Runnable .HasAvailableSubCommands}}
  {{.UseLine}}{{if .HasAvailableSubCommands}} [command]{{end}}{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags | helpFirstFlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
