package cmd

import (
	"fmt"

	"onramp/internal/config"
	"onramp/internal/pscale"
	"onramp/internal/tui"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:     "add",
	Aliases: []string{"a"},
	Short:   "Add a connection string",
	Long: `Add a connection string to your config file. 
By default will use planetscale cli

Optionally, can use --name and --dsn`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dsnFlag, _ := cmd.Flags().GetString("dsn")
		nameFlag, _ := cmd.Flags().GetString("name")
		orgFlag, _ := cmd.Flags().GetString("org")

		if dsnFlag != "" && nameFlag != "" {
			return saveConnection(nameFlag, dsnFlag)
		}
		if dsnFlag != "" {
			fmt.Println("--name is required with --dsn")
			return nil
		}
		if nameFlag != "" {
			fmt.Println("--dsn is required with --name")
			return nil
		}

		orgs, err := pscale.ListOrganizations()
		if err != nil {
			return fmt.Errorf("list organizations: %w", err)
		}
		if len(orgs) == 0 {
			return fmt.Errorf("no organizations found; run 'pscale auth login' first")
		}

		var org, branch, name, dsn string
		var database pscale.Database
		var labels, branches []string
		var dbs []pscale.Database

		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}
		org = cfg.DefaultOrg
		if orgFlag != "" {
			org = orgFlag
		}

		steps := []func() tui.Step{}
		if org == "" {
			steps = append(steps,
				func() tui.Step { return tui.NewSelectStep("Organization", orgs, false, func(s string) { org = s }) },
			)
		}

		steps = append(steps,
			func() tui.Step {
				return tui.NewSpinnerStep("Fetching databases", func() error {
					var err error
					dbs, err = pscale.ListDatabases(org)
					if err != nil {
						return fmt.Errorf("list databases: %w", err)
					}
					if len(dbs) == 0 {
						return fmt.Errorf("no databases found in organization %q", org)
					}
					l := make([]string, len(dbs))
					for i, db := range dbs {
						l[i] = fmt.Sprintf("%s (%s)", db.Name, db.Kind)
					}
					labels = l
					return nil
				})
			},
			func() tui.Step {
				return tui.NewSelectStep("Database", labels, false, func(s string) {
					for _, db := range dbs {
						if fmt.Sprintf("%s (%s)", db.Name, db.Kind) == s {
							database = db
							return
						}
					}
				})
			},
			func() tui.Step {
				return tui.NewSpinnerStep("Fetching branches", func() error {
					var err error
					branches, err = pscale.ListBranches(org, database.Name)
					if err != nil {
						return fmt.Errorf("list branches: %w", err)
					}
					if len(branches) == 0 {
						return fmt.Errorf("no branches found in database %q", database.Name)
					}
					return nil
				})
			},
			func() tui.Step {
				return tui.NewSelectStep("Branch", branches, false, func(s string) {
					branch = s
				})
			},
			func() tui.Step {
				return tui.NewTextInputStep("Connection name/alias:", &name, fmt.Sprintf("pscale-%s-%s", database.Name, branch))
			},
			func() tui.Step {
				return tui.NewSpinnerStep("Creating password for "+org+"/"+database.Name+"/"+branch, func() error {
					var e error
					dsn, e = pscale.CreateDSN(org, database, branch)
					return e
				})
			},
		)

		if err := tui.RunWizard(steps); err != nil {
			return err
		}
		return saveConnection(name, dsn)
	},
}

func saveConnection(name, dsn string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	cfg.Connections[name] = config.Connection{DSN: dsn}

	if cfg.ActiveConnection == "" {
		cfg.ActiveConnection = name
		fmt.Printf("Set %q as the active connection.\n", name)
	}

	if err := config.SaveConfig(cfg); err != nil {
		return err
	}

	provider := config.ProviderFromDSN(dsn)
	fmt.Printf("Connection %q added (provider: %s).\n", name, provider)
	return nil
}

func init() {
	connectionsCmd.AddCommand(addCmd)
	addCmd.Flags().String("dsn", "", "Database connection string")
	addCmd.Flags().String("name", "", "Alias to identify the connection")
	addCmd.Flags().String("org", "", "Organization database is in")
}
