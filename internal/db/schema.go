// Package db
package db

import (
	"context"
	"database/sql"
	"fmt"
)

// DropAllTables drops every base table in the connected database.
// For MySQL it disables FK checks to allow dropping in any order.
// For Postgres it uses CASCADE.
func DropAllTables(ctx context.Context, sqlDB *sql.DB, provider string) error {
	switch provider {
	case "postgres":
		rows, err := sqlDB.QueryContext(ctx,
			WithAction(`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`, "ddl"))
		if err != nil {
			return fmt.Errorf("list tables: %w", err)
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				return fmt.Errorf("scan table name: %w", err)
			}
			tables = append(tables, t)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate tables: %w", err)
		}

		for _, t := range tables {
			if _, err := sqlDB.ExecContext(ctx, WithAction(`DROP TABLE IF EXISTS "`+t+`" CASCADE`, "ddl")); err != nil {
				return fmt.Errorf("drop table %q: %w", t, err)
			}
			fmt.Printf("  dropped %s\n", t)
		}

	default: // mysql / vitess
		if _, err := sqlDB.ExecContext(ctx, WithAction(`SET FOREIGN_KEY_CHECKS = 0`, "ddl")); err != nil {
			return fmt.Errorf("disable FK checks: %w", err)
		}

		rows, err := sqlDB.QueryContext(ctx,
			WithAction(`SELECT table_name FROM information_schema.tables
			 WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'`, "ddl"))
		if err != nil {
			return fmt.Errorf("list tables: %w", err)
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				return fmt.Errorf("scan table name: %w", err)
			}
			tables = append(tables, t)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate tables: %w", err)
		}

		for _, t := range tables {
			if _, err := sqlDB.ExecContext(ctx, WithAction(`DROP TABLE IF EXISTS `+"`"+t+"`", "ddl")); err != nil {
				return fmt.Errorf("drop table %q: %w", t, err)
			}
			fmt.Printf("  dropped %s\n", t)
		}

		if _, err := sqlDB.ExecContext(ctx, WithAction(`SET FOREIGN_KEY_CHECKS = 1`, "ddl")); err != nil {
			return fmt.Errorf("re-enable FK checks: %w", err)
		}
	}
	return nil
}

// ApplySchema executes each DDL statement in stmts against the database.
func ApplySchema(ctx context.Context, sqlDB *sql.DB, stmts []string) error {
	for _, stmt := range stmts {
		if _, err := sqlDB.ExecContext(ctx, WithAction(stmt, "ddl")); err != nil {
			return fmt.Errorf("apply schema statement: %w", err)
		}
	}
	fmt.Printf("Applied %d schema statements.\n", len(stmts))
	return nil
}

// ResetSchema drops all tables and then applies stmts.
func ResetSchema(ctx context.Context, sqlDB *sql.DB, provider string, stmts []string) error {
	if err := DropAllTables(ctx, sqlDB, provider); err != nil {
		return fmt.Errorf("drop all tables: %w", err)
	}
	return ApplySchema(ctx, sqlDB, stmts)
}
