package cmd

import (
	"fmt"

	"onramp/internal/config"

	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove [name]",
	Aliases: []string{"r"},
	Short: "Remove a saved connection string",
	Long:  `Remove a saved connection string by name.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}

		if _, ok := cfg.Connections[name]; !ok {
			return fmt.Errorf("connection %q not found", name)
		}

		wasActive := cfg.ActiveConnection == name
		delete(cfg.Connections, name)
		if wasActive {
			cfg.ActiveConnection = ""
		}

		if err := config.SaveConfig(cfg); err != nil {
			return err
		}

		fmt.Printf("Connection %q removed.\n", name)
		if wasActive {
			fmt.Println("Warning: this was the active connection. Run 'onramp connections select' to set a new one.")
		}
		return nil
	},
}

func init() {
	connectionsCmd.AddCommand(removeCmd)
}
