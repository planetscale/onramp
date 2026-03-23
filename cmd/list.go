package cmd

import (
	"fmt"
	"net/url"
	"sort"

	"onramp/internal/config"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "List all db connection strings saved",
	Long:    `List all db connection strings saved. Only shows name and username, never passwords.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}

		if len(cfg.Connections) == 0 {
			fmt.Println("No connections saved.")
			return nil
		}

		names := make([]string, 0, len(cfg.Connections))
		for name := range cfg.Connections {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			prefix := "  "
			if name == cfg.ActiveConnection {
				prefix = "* "
			}
			fmt.Printf("%s%s\n", prefix, name)
		}
		return nil
	},
}

func userFromDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		return ""
	}
	return u.User.Username()
}

func init() {
	connectionsCmd.AddCommand(listCmd)
}
