package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"

	"onramp/internal/config"
	"onramp/internal/db"
	"onramp/internal/hammer"
	"onramp/internal/serve"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve onramp HTTP Server",
	Long:  `Starts the onramp HTTP server. Prints the API key to stdout on startup.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		skipInit, _ := cmd.Flags().GetBool("skip-init")
		maxCons, _ := cmd.Flags().GetInt("max-cons")
		interactive, _ := cmd.Flags().GetBool("interactive")
		useSSH, _ := cmd.Flags().GetBool("ssh")

		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		conn, err := config.GetActiveConnection(cfg)
		if err != nil {
			return err
		}

		provider := config.ProviderFromDSN(conn.DSN)
		sqlDB, err := sql.Open(provider, conn.DSN)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		sqlDB.SetMaxOpenConns(maxCons)
		sqlDB.SetMaxIdleConns(maxCons)
		defer sqlDB.Close()

		if err := sqlDB.Ping(); err != nil {
			return fmt.Errorf("connect to db: %w", err)
		}

		if !skipInit {
			stmts := config.GetSchemaFromConfig(cfg, provider)
			if err := db.ResetSchema(context.Background(), sqlDB, provider, stmts); err != nil {
				return fmt.Errorf("reset schema: %w", err)
			}
		}

		apiKey, err := serve.RandHex(32)
		if err != nil {
			return fmt.Errorf("generate api key: %w", err)
		}
		fmt.Printf("API Key: %s\n", apiKey)

		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		poasterDir := home + "/.poaster"

		var bunCmd *exec.Cmd
		var bunPort int
		if interactive {
			if _, err := os.Stat(poasterDir); os.IsNotExist(err) {
				repoURL := "https://github.com/planetscale/poaster"
				if useSSH {
					repoURL = "git@github.com:planetscale/poaster.git"
				}
				clone := exec.Command("git", "clone", repoURL, poasterDir)
				clone.Stdout = os.Stdout
				clone.Stderr = os.Stderr
				if err := clone.Run(); err != nil {
					return fmt.Errorf("clone poaster: %w", err)
				}
			}

			install := exec.Command("bun", "i")
			install.Dir = poasterDir
			install.Stdout = os.Stdout
			install.Stderr = os.Stderr
			if err := install.Run(); err != nil {
				return fmt.Errorf("bun i: %w", err)
			}

			bunPort = 3000
			bunCmd = exec.Command("bun", "run", "dev", "--open", "--port", fmt.Sprintf("%d", bunPort))
			bunCmd.Dir = poasterDir
			bunCmd.Stdout = os.Stdout
			bunCmd.Stderr = os.Stderr
			bunCmd.Env = append(os.Environ(),
				"API_KEY="+apiKey,
				"DATABASE_URL="+conn.DSN,
				"ONRAMP_URL="+fmt.Sprintf("http://localhost:%d", port),
			)
			if err := bunCmd.Start(); err != nil {
				return fmt.Errorf("start bun dev: %w", err)
			}
		}

		state := config.GetSimFromConfig(cfg, provider)

		engine := hammer.NewEngine(state, sqlDB, provider)
		state.SetRunning(true)
		go engine.Run()
		go engine.Start()

		e := serve.NewServer(state, apiKey, engine)

		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

		wg := sync.WaitGroup{}
		wg.Go(func() {
			<-quit
			fmt.Println("\nShutting down...")
			engine.Shutdown()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = e.Shutdown(ctx)
			if bunCmd != nil && bunCmd.Process != nil {
				_ = bunCmd.Process.Signal(os.Interrupt)
				_ = bunCmd.Wait()
			}
		})

		addr := fmt.Sprintf(":%d", port)
		fmt.Printf("onramp API listening on http://localhost%s\n", addr)
		if interactive {
			fmt.Printf("onramp UI serving on http://localhost:%d\n", bunPort)
		}
		e.Start(addr)
		wg.Wait()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().Int("port", 8080, "HTTP port to listen on")
	serveCmd.Flags().Bool("skip-init", false, "Skips initialization of db schema")
	serveCmd.Flags().Int("max-cons", 8, "Max connections to db")
	serveCmd.Flags().Bool("interactive", false, "Also start the Poaster UI dev server (runs bun i then bun run dev in ./poaster)")
	serveCmd.Flags().Bool("ssh", false, "Clone the poaster repo using SSH instead of HTTPS")
}
