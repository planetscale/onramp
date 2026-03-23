package cmd

import (
	"fmt"
	"sort"
	"strings"

	"onramp/internal/config"
	"onramp/internal/tui"

	"github.com/spf13/cobra"
)

var selectCmd = &cobra.Command{
	Use:     "select [name]",
	Aliases: []string{"sel"},
	Short:   "Select a db connection to use",
	Long:    `Selects a db connection to use for future commands. If no name is given, shows an interactive prompt.`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}

		if len(cfg.Connections) == 0 {
			return fmt.Errorf("no connections saved; run 'onramp connections add' first")
		}

		var name string
		if len(args) == 1 {
			name = args[0]
		} else {
			names := make([]string, 0, len(cfg.Connections))
			for n := range cfg.Connections {
				if n == cfg.ActiveConnection {
					n = n + " (current)"
				}
				names = append(names, n)
			}
			sort.Strings(names)

			if err := tui.RunWizard([]func() tui.Step{func() tui.Step { return tui.NewSelectStep("connection", names, false, func(s string) { name = s }) }}); err != nil {
				return err
			}
		}
		name, _ = strings.CutSuffix(name, " (current)")
		if _, ok := cfg.Connections[name]; !ok {
			return fmt.Errorf("connection %q not found", name)
		}

		cfg.ActiveConnection = name
		if err := config.SaveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Active connection set to %q.\n", name)
		return nil
	},
}

func init() {
	connectionsCmd.AddCommand(selectCmd)
}
