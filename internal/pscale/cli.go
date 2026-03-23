// Package pscale wraps the pscale CLI to list resources and create passwords.
package pscale

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"onramp/internal/serve"
)

// run executes a pscale command with --format json and decodes the output into out.
func run(out any, args ...string) error {
	args = append(args, "--format", "json")
	cmd := exec.Command("pscale", args...)
	data, err := cmd.Output()
	if err != nil {
		// pscale writes errors as JSON to stdout; try to surface the message.
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s", ee.Stderr)
		}
		return err
	}
	return json.Unmarshal(data, out)
}

// ListOrganizations returns the names of all organizations the current user can access.
func ListOrganizations() ([]string, error) {
	var rows []struct {
		Name string `json:"name"`
	}
	if err := run(&rows, "org", "list"); err != nil {
		return nil, err
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	return names, nil
}

type Database struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ListDatabases returns the names of all databases in the given organization.
func ListDatabases(org string) ([]Database, error) {
	var rows []Database
	if err := run(&rows, "database", "list", "--org", org); err != nil {
		return nil, err
	}
	return rows, nil
}

// ListBranches returns the names of all branches in the given database.
func ListBranches(org, database string) ([]string, error) {
	var rows []struct {
		Name string `json:"name"`
	}
	if err := run(&rows, "branch", "list", database, "--org", org); err != nil {
		return nil, err
	}
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = r.Name
	}
	return names, nil
}

// Password holds the credentials returned when creating a branch password.
type Password struct {
	Username      string `json:"username"`
	PlainText     string `json:"plain_text"`
	AccessHostURL string `json:"access_host_url"`
	PostgresURL   string `json:"database_url"`
}

// CreateDatabaseOptions holds optional flags forwarded to pscale database create.
type CreateDatabaseOptions struct {
	Engine   string // "mysql" or "postgresql"
	Metal    bool
	Region   string
	Size     string
	Replicas int // 0 = single node, 2+ = HA; only used for PostgreSQL
}

type Status struct {
	State string `json:"state"`
}

func DatabaseStatus(org, name string) (string, error) {
	args := []string{"database", "show", name, "--org", org}
	status := Status{}
	if err := run(&status, args...); err != nil {
		return "", err
	}
	return status.State, nil
}

type Rules struct {
	Kind        string `json:"kind,omitempty"`        // Kind of rule
	Keyspace    string `json:"keyspace,omitempty"`    // Keyspace to apply rule to, required: false
	Fingerprint string `json:"fingerprint,omitempty"` // Query fingerprint to apply rule to, required: false
	Tags        []Tags `json:"tags,omitempty"`        // Optional array of tags for this rule. When provided, creates multiple tags per rule instead of a single tag from key/key_source/value., required: false do
}

type Tags struct {
	Key    string `json:"key,omitempty"`    // Tag key
	Value  string `json:"value,omitempty"`  // Tag value
	Source string `json:"source,omitempty"` // Source of the tag
}

type TrafficControlRule struct {
	Name        string  `json:"name,omitempty"`        // Name of the traffic budget
	Mode        string  `json:"mode,omitempty"`        // ["enforce", "warn", "off"], The mode of the traffic budget
	Capacity    int     `json:"capacity,omitempty"`    // The maximum capacity a single query can consume, measured in seconds of full server usage
	Rate        int     `json:"rate,omitempty"`        // Rate limit percentage (0-100)
	Burst       int     `json:"burst,omitempty"`       // Burst maximum capacity that can be banked, measured in seconds of full server usage
	Concurrency int     `json:"concurrency,omitempty"` // Concurrency limit percentage (0-100)
	Rules       []Rules `json:"rules,omitempty"`       // Array of traffic rules to apply to the budget
}

func CreateTCRules(org, db, branch string, tcr TrafficControlRule) error {
	args := []string{"api", "-X", "POST", fmt.Sprintf("/organizations/%s/databases/%s/branches/%s/traffic/budgets", org, db, branch), "--format", "json", "--input=-"}
	cmd := exec.Command("pscale", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	jsonData, err := json.Marshal(tcr)
	if err != nil {
		return err
	}

	var e error
	// Start a goroutine to write the JSON data to stdin and close the pipe
	go func() {
		defer stdin.Close() // Crucial: close the pipe when done writing
		if _, err := io.Writer.Write(stdin, jsonData); err != nil {
			e = err
		}
	}()
	data, err := cmd.Output()
	if e != nil {
		return e
	}
	if err != nil {
		// pscale writes errors as JSON to stdout; try to surface the message.
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("%s", e.Error)
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s", ee.Stderr)
		}
		return err
	}

	return nil
}

// CreateDatabase creates a new database in the given organization and returns it.
func CreateDatabase(org, name string, opts CreateDatabaseOptions) (Database, error) {
	args := []string{"database", "create", name, "--org", org}
	if opts.Engine != "" {
		args = append(args, "--engine", opts.Engine)
	}
	if opts.Metal {
		args = append(args, "--metal")
	}
	if opts.Region != "" {
		args = append(args, "--region", opts.Region)
	}
	if opts.Size != "" {
		args = append(args, "--cluster-size", opts.Size)
	}
	if opts.Engine == "postgresql" {
		args = append(args, "--replicas", fmt.Sprintf("%d", opts.Replicas))
	}
	var db Database
	if err := run(&db, args...); err != nil {
		return Database{}, err
	}
	return db, nil
}

// ClusterSize holds metadata for a single cluster size option.
type ClusterSize struct {
	Name        string // e.g. PS_5_AWS_X86
	DisplayName string // e.g. PS-5
	Label       string // human-readable label for the picker
	CPU         string
	RAM         int64
	Rate        float64 // HA price ($/mo)
	ReplicaRate float64 // single-node price ($/mo)
	Replicas    int     // 0 = single node, 2 = HA
}

// ListClusterSizes returns the available cluster sizes for the given organization.
func ListClusterSizes(org string, engine string) ([]ClusterSize, error) {
	var rows []struct {
		Name        string  `json:"name"`
		DisplayName string  `json:"display_name"`
		CPU         string  `json:"cpu"`
		RAM         int64   `json:"ram"`
		Rate        float64 `json:"rate"`
		ReplicaRate float64 `json:"replica_rate"`
	}
	if err := run(&rows, "size", "cluster", "list", "--org", org, "--engine", engine); err != nil {
		return nil, err
	}

	type key struct {
		Name     string
		Replicas int
	}
	seen := map[key]bool{}
	var sizes []ClusterSize

	for _, r := range rows {
		ram := formatRAM(r.RAM)

		// HA option (rate)
		if r.Rate > 0 {
			k := key{r.Name, 2}
			if !seen[k] {
				seen[k] = true
				sizes = append(sizes, ClusterSize{
					Name:        r.Name,
					DisplayName: r.DisplayName,
					Label:       fmt.Sprintf("%s - High Availability (%s vCPU, %s, $%.0f/mo)", r.Name, r.CPU, ram, r.Rate),
					CPU:         r.CPU,
					RAM:         r.RAM,
					Rate:        r.Rate,
					ReplicaRate: r.ReplicaRate,
					Replicas:    2,
				})
			}
		}

		// Single-node option (replica_rate) — only for Postgres
		if r.ReplicaRate > 0 && engine == "postgresql" {
			k := key{r.Name, 0}
			if !seen[k] {
				seen[k] = true
				sizes = append(sizes, ClusterSize{
					Name:        r.Name,
					DisplayName: r.DisplayName,
					Label:       fmt.Sprintf("%s - Single Node (%s vCPU, %s, $%.0f/mo)", r.Name, r.CPU, ram, r.ReplicaRate),
					CPU:         r.CPU,
					RAM:         r.RAM,
					Rate:        r.Rate,
					ReplicaRate: r.ReplicaRate,
					Replicas:    0,
				})
			}
		}
	}

	return sizes, nil
}

// formatRAM converts bytes to a human-readable string.
func formatRAM(bytes int64) string {
	const (
		mb = 1 << 20
		gb = 1 << 30
		tb = 1 << 40
	)
	switch {
	case bytes >= tb:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%d GB", bytes/gb)
	default:
		return fmt.Sprintf("%d MB", bytes/mb)
	}
}

// CreateDSN creates a new readwriter password for the given branch.
// It tries pscale password create first; if the database is PostgreSQL it
// falls back to pscale api POST which supports both engine types.
func CreateDSN(org string, database Database, branch string) (string, error) {
	var pw Password
	suffix, _ := serve.RandHex(8)
	switch database.Kind {
	case "mysql":
		err := run(&pw, "password", "create",
			database.Name, branch, fmt.Sprintf("onramp-%s", suffix),
			"--org", org,
			"--role", "admin",
		)
		if err != nil {
			return "", err
		}
		dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?tls=true&parseTime=true",
			pw.Username, pw.PlainText, pw.AccessHostURL, database.Name)
		return dsn, nil

	case "postgresql":
		err := run(&pw, "role", "create",
			database.Name, branch, fmt.Sprintf("onramp-%s", suffix),
			"--org", org,
			"--inherited-roles", "postgres",
		)
		if err != nil {
			return "", err
		}
		return pw.PostgresURL, nil
	}
	return "", errors.New("Unknown db type")
}
