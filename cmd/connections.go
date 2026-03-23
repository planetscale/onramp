package cmd

import (
	"github.com/spf13/cobra"
)

// connectionsCmd represents the connections command
var connectionsCmd = &cobra.Command{
	Use:     "connections",
	Aliases: []string{"con"},
	Short:   "Manage database connections",
	Long:    `Add, List and Remove connections`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(connectionsCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// connectionsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// connectionsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
