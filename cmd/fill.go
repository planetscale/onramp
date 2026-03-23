package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"onramp/internal/config"
	"onramp/internal/db"
	"onramp/internal/tui"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

var fillCmd = &cobra.Command{
	Use:   "fill",
	Short: "Fill database with dummy data",
	Long:  `Populate the initialized database with dummy data up to a row count or size target.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		rows, _ := cmd.Flags().GetInt("rows")
		size, _ := cmd.Flags().GetInt("size")
		skipInit, _ := cmd.Flags().GetBool("skip-init")
		maxCons, _ := cmd.Flags().GetInt("max-cons")

		if rows != 0 && size != 0 {
			return errors.New("--rows and --size are mutually exclusive")
		}
		if rows == 0 && size == 0 {
			return errors.New("one of --rows or --size must be provided")
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			return err
		}

		conn, err := config.GetActiveConnection(cfg)
		if err != nil {
			return err
		}

		provider := config.ProviderFromDSN(conn.DSN)
		fmt.Printf("Connecting to %s database...\n", provider)

		sqlDB, err := sql.Open(provider, conn.DSN)
		if err != nil {
			return fmt.Errorf("failed to open connection: %w", err)
		}
		sqlDB.SetMaxOpenConns(maxCons)
		sqlDB.SetMaxIdleConns(maxCons)

		defer sqlDB.Close()

		if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		if !skipInit {
			stmts := config.GetSchemaFromConfig(cfg, provider)
			if err := db.ResetSchema(context.Background(), sqlDB, provider, stmts); err != nil {
				return fmt.Errorf("reset schema: %w", err)
			}
		}

		p := &tui.FillProgress{TargetGB: size, Title: "Initializing..."}
		return tui.RunFillSpinner(p, func() error {
			mu := sync.Mutex{}
			return db.Fill(context.Background(), sqlDB, provider, db.FillInput{
				Rows:     rows,
				TargetGB: size,
				OnBatch:  func(n int) { p.Rows.Add(int64(n)) },
				OnBytes:  func(b int64) { p.Bytes.Store(b) },
				SetTitle: func(s string) {
					mu.Lock()
					defer mu.Unlock()
					p.Title = s
				},
			})
		})
	},
}

func init() {
	rootCmd.AddCommand(fillCmd)
	fillCmd.Flags().Bool("skip-init", false, "Skips initialization of db schema")
	fillCmd.Flags().Int("rows", 0, "Rows to insert per table")
	fillCmd.Flags().Int("size", 0, "Target database size in GB")
	fillCmd.Flags().Int("max-cons", 16, "Max connections to db")
}
