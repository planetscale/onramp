package cmd

import (
	"fmt"
	"time"

	"onramp/internal/config"
	"onramp/internal/pscale"
	"onramp/internal/tui"

	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"c"},
	Short:   "Create a new database on planetscale",
	Long:    `Create a new testing database on planetscale`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}
		metal, _ := cmd.Flags().GetBool("metal")
		region, _ := cmd.Flags().GetString("region")
		engineFlag, _ := cmd.Flags().GetString("engine")
		orgFlag, _ := cmd.Flags().GetString("org")

		var org, name, engine, size, conName string
		var tc bool
		var replicas int
		var sizes []pscale.ClusterSize
		var orgs []string

		if engineFlag != "" {
			engine = engineFlag
		}
		org = cfg.DefaultOrg
		if orgFlag != "" {
			org = orgFlag
		}

		steps := []func() tui.Step{}

		if org == "" {
			steps = append(steps, func() tui.Step {
				return tui.NewSpinnerStep("Fetching orgs", func() error {
					var err error
					orgs, err = pscale.ListOrganizations()
					if err != nil {
						return fmt.Errorf("list organizations: %w", err)
					}
					if len(orgs) == 0 {
						return fmt.Errorf("no organizations found; run 'pscale auth login' first")
					}
					return nil
				})
			},
				func() tui.Step {
					return tui.NewSelectStep("Organization", orgs, false, func(s string) { org = s })
				},
			)
		}

		steps = append(steps,
			func() tui.Step {
				return tui.NewTextInputStep("New database name:", &name, "")
			},
		)
		if engineFlag == "" {
			steps = append(steps, func() tui.Step {
				return tui.NewSelectStep("Engine", []string{"mysql", "postgresql"}, false, func(s string) { engine = s })
			})
		}
		steps = append(steps,
			func() tui.Step {
				if engine == "mysql" {
					return nil
				}
				return tui.NewSelectStep("Enable Traffic Control?", []string{"Yes", "No"}, false, func(s string) {
					if s == "Yes" {
						tc = true
					}
				})
			},
			func() tui.Step {
				return tui.NewSpinnerStep("Fetching cluster sizes", func() error {
					var err error
					sizes, err = pscale.ListClusterSizes(org, engine)
					if err != nil {
						return fmt.Errorf("list cluster sizes: %w", err)
					}
					return nil
				})
			},
			func() tui.Step {
				labels := make([]string, len(sizes))
				for i, s := range sizes {
					labels[i] = s.Label
				}
				return tui.NewSelectStep("Cluster size", labels, true, func(s string) {
					for _, cs := range sizes {
						if cs.Label == s {
							size = cs.Name
							replicas = cs.Replicas
							break
						}
					}
				})
			},
			func() tui.Step {
				return tui.NewTextInputStep("Connection name/alias:", &conName, fmt.Sprintf("pscale-%s-%s", name, "main"))
			},
			func() tui.Step {
				return tui.NewSpinnerStep(`Creating database "`+name+`"`, func() error {
					_, e := pscale.CreateDatabase(org, name, pscale.CreateDatabaseOptions{
						Engine:   engine,
						Metal:    metal,
						Region:   region,
						Size:     size,
						Replicas: replicas,
					})
					return e
				})
			},
			func() tui.Step {
				return tui.NewSpinnerStep(`Initializing database "`+name+`"`, func() error {
					for {
						status, e := pscale.DatabaseStatus(org, name)
						if status == "ready" {
							break
						}
						if err != nil {
							return e
						}
						time.Sleep(2 * time.Second)
					}
					return nil
				})
			},
			func() tui.Step {
				if !tc {
					return nil
				}
				defaultTCRules := config.DefaultTCRules()
				for _, r := range defaultTCRules {
					pscale.CreateTCRules(org, name, "main", r)
				}
				return nil
			},
			func() tui.Step {
				return tui.NewSpinnerStep("Creating password for "+org+"/"+name+"/"+"main", func() error {
					dsn, e := pscale.CreateDSN(org, pscale.Database{Name: name, Kind: engine}, "main")
					if err != nil {
						return e
					}
					saveConnection(conName, dsn)
					return nil
				})
			},
		)

		if err := tui.RunWizard(steps); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().String("engine", "", "Database engine: mysql or postgresql")
	createCmd.Flags().Bool("metal", false, "Use PlanetScale Metal (dedicated hardware)")
	createCmd.Flags().String("region", "", "Region slug for the database")
	createCmd.Flags().String("org", "", "Organization to create database in")
}
