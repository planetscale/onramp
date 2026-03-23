package cmd

import (
	"fmt"

	"onramp/internal/config"
	"onramp/internal/pscale"
	"onramp/internal/tui"

	"github.com/spf13/cobra"
)

// defaultOrgCmd represents the defaultOrg command
var defaultOrgCmd = &cobra.Command{
	Use:   "default-org",
	Short: "Selects the users default org",
	Long:  `Selects the users default org`,
	RunE: func(cmd *cobra.Command, args []string) error {
		orgs, err := pscale.ListOrganizations()
		if err != nil {
			return fmt.Errorf("list organizations: %w", err)
		}
		if len(orgs) == 0 {
			return fmt.Errorf("no organizations found; run 'pscale auth login' first")
		}
		var org string
		steps := []func() tui.Step{
			func() tui.Step {
				return tui.NewSelectStep("organization", orgs, false, func(s string) { org = s })
			},
		}
		if err := tui.RunWizard(steps); err != nil {
			return err
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}
		cfg.DefaultOrg = org
		err = config.SaveConfig(cfg)
		return err
	},
}

func init() {
	rootCmd.AddCommand(defaultOrgCmd)
}
