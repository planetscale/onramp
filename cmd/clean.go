package cmd

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"onramp/internal/config"
	"onramp/internal/db"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Drop all tables in the database",
	Long:  `Drop all tables in the active database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}

		conn, err := config.GetActiveConnection(cfg)
		if err != nil {
			return err
		}

		if !force {
			fmt.Printf("This will drop all tables on %s.\n", cfg.ActiveConnection)
			fmt.Print("Type 'yes' to confirm: ")
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			if strings.TrimSpace(scanner.Text()) != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		provider := config.ProviderFromDSN(conn.DSN)
		fmt.Printf("Connecting to %s database...\n", provider)
		sqlDB, err := sql.Open(provider, conn.DSN)
		if err != nil {
			return fmt.Errorf("failed to open connection: %w", err)
		}
		defer sqlDB.Close()

		if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		fmt.Println("Dropping tables...")
		if err := db.DropAllTables(context.Background(), sqlDB, provider); err != nil {
			return err
		}
		fmt.Println("Done.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	cleanCmd.Flags().Bool("force", false, "Skip confirmation prompt and drop immediately")
}
