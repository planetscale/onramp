// Package config
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"onramp/internal/hammer"
	"onramp/internal/hammer/htypes"
	"onramp/internal/pscale"

	"go.yaml.in/yaml/v3"
)

type Connection struct {
	DSN string `yaml:"dsn"`
}

type Config struct {
	ActiveConnection string                `yaml:"active-connection,omitempty"`
	Connections      map[string]Connection `yaml:"connections,omitempty"`
	DefaultOrg       string                `yaml:"default-org"`
	Hammer           struct {
		Mysql    *htypes.UserConfig `yaml:"mysql,omitempty"`
		Postgres *htypes.UserConfig `yaml:"postgres,omitempty"`
	}
	Schema struct {
		Mysql    []string `yaml:"mysql,omitempty"`
		Postgres []string `yaml:"postgres,omitempty"`
	} `yaml:"schema,omitempty"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".onramp.yaml"), nil
}

func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Connections: make(map[string]Connection)}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Connections == nil {
		cfg.Connections = make(map[string]Connection)
	}
	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func GetActiveConnection(cfg *Config) (*Connection, error) {
	if cfg.ActiveConnection == "" {
		return nil, fmt.Errorf("no active connection set; run 'onramp connections select'")
	}
	conn, ok := cfg.Connections[cfg.ActiveConnection]
	if !ok {
		return nil, fmt.Errorf("active connection %q not found in config", cfg.ActiveConnection)
	}
	return &conn, nil
}

// ProviderFromDSN returns "postgres" or "mysql" based on the DSN scheme.
func ProviderFromDSN(dsn string) string {
	u, err := url.Parse(dsn)
	// Assume non urls are vitess
	if err != nil {
		return "mysql"
	}
	switch u.Scheme {
	case "postgres", "postgresql":
		return "postgres"
	default:
		return "mysql"
	}
}

func GetSimFromConfig(cfg *Config, provider string) *hammer.SimulationState {
	defaults := &hammer.SimulationState{
		Provider: provider,
		UserConfig: htypes.UserConfig{
			NewUsersPerSecond:     1,
			DeletedUsersPerSecond: 0.5,
			UserFlowDelay:         500 * time.Millisecond,
			MaxUsers:              50,
			MinUsers:              0,
			Flows:                 DefaultFlows(provider),
		},
	}

	switch provider {
	case "mysql":
		if cfg.Hammer.Mysql != nil {
			defaults.UserConfig = *cfg.Hammer.Mysql
		}
		cfg.Hammer.Mysql = &defaults.UserConfig
	case "postgres":
		if cfg.Hammer.Postgres != nil {
			defaults.UserConfig = *cfg.Hammer.Postgres
		}
		cfg.Hammer.Postgres = &defaults.UserConfig
	}
	SaveConfig(cfg)
	return defaults
}

// DefaultSchema returns the default DDL statements for the given provider.
func DefaultSchema(provider string) []string {
	if provider == "postgres" {
		return []string{
			`CREATE TABLE IF NOT EXISTS users (
				id         BIGSERIAL PRIMARY KEY,
				name       VARCHAR(255) NOT NULL,
				email      VARCHAR(255) NOT NULL,
				created_at TIMESTAMPTZ DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS posts (
				id         BIGSERIAL PRIMARY KEY,
				user_id    BIGINT NOT NULL,
				body       TEXT,
				created_at TIMESTAMPTZ DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS comments (
				id         BIGSERIAL PRIMARY KEY,
				post_id    BIGINT NOT NULL,
				user_id    BIGINT NOT NULL,
				body       TEXT,
				created_at TIMESTAMPTZ DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS tags (
				id   BIGSERIAL PRIMARY KEY,
				name VARCHAR(100) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS post_tags (
				post_id BIGINT NOT NULL,
				tag_id  BIGINT NOT NULL,
				PRIMARY KEY (post_id, tag_id)
			)`,
			`CREATE TABLE activity (
				id SERIAL PRIMARY KEY,
				user_id INTEGER NOT NULL,
				action TEXT NOT NULL,
				target_type TEXT NOT NULL,
				target_id INTEGER,
				metadata JSONB DEFAULT '{}',
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE report_snapshots (
				id SERIAL PRIMARY KEY,
				name TEXT NOT NULL
			)`,
			`CREATE INDEX idx_activity_user ON activity (user_id, created_at)`,
			`CREATE INDEX idx_activity_action ON activity (action, created_at)`,
			`CREATE INDEX idx_posts_created_at ON posts (created_at)`,
			`CREATE INDEX idx_posts_user_id ON posts (user_id)`,
			`CREATE INDEX idx_comments_post_id_created_at ON comments (post_id, created_at)`,
			`CREATE INDEX idx_comments_created_at ON comments (created_at)`,
		}
	}

	// MySQL / Vitess
	return []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name       VARCHAR(255) NOT NULL,
			email      VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			user_id    BIGINT NOT NULL,
			body       TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS comments (
			id         BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			post_id    BIGINT NOT NULL,
			user_id    BIGINT NOT NULL,
			body       TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(100) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS post_tags (
			post_id BIGINT NOT NULL,
			tag_id  BIGINT NOT NULL,
			PRIMARY KEY (post_id, tag_id)
		)`,
	}
}

// GetSchemaFromConfig returns the schema statements for the given provider,
// writing defaults back to cfg and saving if not already set.
func GetSchemaFromConfig(cfg *Config, provider string) []string {
	defaults := DefaultSchema(provider)
	switch provider {
	case "mysql":
		if len(cfg.Schema.Mysql) > 0 {
			defaults = cfg.Schema.Mysql
		}
		cfg.Schema.Mysql = defaults
	case "postgres":
		if len(cfg.Schema.Postgres) > 0 {
			defaults = cfg.Schema.Postgres
		}
		cfg.Schema.Postgres = defaults
	}
	SaveConfig(cfg)
	return defaults
}

// @simeon
// Add TC rules and default queries for inital demo below

// DefaultTCRules returns the default TC rules
func DefaultTCRules() []pscale.TrafficControlRule {
	return []pscale.TrafficControlRule{
		{
			Name:        "Delete User",
			Mode:        "enforce",
			Capacity:    100,
			Rate:        100,
			Burst:       100,
			Concurrency: 10,
			Rules: []pscale.Rules{{
				Kind: "match",
				Tags: []pscale.Tags{{
					Key:    "action",
					Value:  "delete_user",
					Source: "sql",
				}},
			}},
		},
		{
			Name:        "Create User",
			Mode:        "enforce",
			Capacity:    10,
			Rate:        10,
			Burst:       20,
			Concurrency: 10,
			Rules: []pscale.Rules{{
				Kind: "match",
				Tags: []pscale.Tags{{
					Key:    "action",
					Value:  "new_user",
					Source: "sql",
				}},
			}},
		},
	}
}

// DefaultFlows returns three hard-coded default flows.
func DefaultFlows(provider string) []htypes.Flow {
	randomFunc := "RANDOM()"
	if provider != "postgres" {
		randomFunc = "RAND()"
	}
	//

	flows := []htypes.Flow{
		{
			Name:   "create_post",
			Weight: 40,
			Queries: []string{
				`INSERT INTO posts (user_id, body) VALUES ({{.UserID}}, '{{Sentence 15}}') /*action='new_post'*/`,
			},
		},
		{
			Name:   "create_comment",
			Weight: 40,
			Queries: []string{
				// INSERT ... SELECT is a no-op when posts is empty, avoiding a NULL post_id error.
				fmt.Sprintf(
					`INSERT INTO comments (post_id, user_id, body) SELECT id, {{.UserID}}, '{{Sentence 8}}' FROM posts ORDER BY %s LIMIT 1 /*action='new_comment'*/`,
					randomFunc,
				),
			},
		},
		{
			Name:   "delete_comment",
			Weight: 20,
			Queries: []string{
				// Wrap inner SELECT in a derived table so MySQL/Vitess allows deleting from the same table.
				fmt.Sprintf(
					`DELETE FROM comments WHERE id = (SELECT id FROM (SELECT id FROM comments WHERE user_id = {{.UserID}} ORDER BY %s LIMIT 1) AS t) /*action='remove_comment'*/`,
					randomFunc,
				),
			},
		},
	}

	if provider == "mysql" {
		flows = append(flows, htypes.Flow{
			Name:   "create_post_tag",
			Weight: 5,
			Queries: []string{
				`INSERT IGNORE INTO post_tags (post_id, tag_id) SELECT p.id, t.id FROM posts p CROSS JOIN tags t WHERE p.user_id = {{.UserID}} AND (SELECT COUNT(*) FROM post_tags pt WHERE pt.post_id = p.id) < 4 ORDER BY RAND(), RAND() LIMIT 1 /*action='create_post_tag'*/`,
			},
		},
			htypes.Flow{
				Name:   "create_tag",
				Weight: 10,
				Queries: []string{
					`INSERT IGNORE INTO tags (name) VALUES ('{{Word}}') /*action='create_tag'*/`,
				},
			},
		)
	}

	if provider == "postgres" {
		flows = append(flows, htypes.Flow{
			Name:   "create_post_tag",
			Weight: 5,
			Queries: []string{
				`INSERT INTO post_tags (post_id, tag_id) SELECT p.id, t.id FROM posts p CROSS JOIN tags t WHERE p.user_id = {{.UserID}} AND (SELECT COUNT(*) FROM post_tags pt WHERE pt.post_id = p.id) < 4 ORDER BY RANDOM(), RANDOM() LIMIT 1 ON CONFLICT DO NOTHING /*action='create_post_tag'*/`,
			},
		},
			htypes.Flow{
				Name:   "create_tag",
				Weight: 10,
				Queries: []string{
					`INSERT INTO tags (name) VALUES ('{{Word}}') ON CONFLICT (name) DO NOTHING /*action='create_tag'*/`,
				},
			},
		)
	}
	//

	return flows
}
