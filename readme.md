# onramp

A CLI tool for bootstrapping databases with dummy data and hammering them with
configurable query loads. Useful for load testing, schema validation, and
simulating realistic database traffic.

Supports **MySQL** and **PostgreSQL**, with built-in integration for
[PlanetScale](https://planetscale.com).

# IMPORTANT

This tool **IS DESTRUCTIVE**. Most of the commands **WILL** wipe all tables and
data in your database. This tool is for demoing and testing out new databases.

Make sure you have pscale cli installed and authenticated with
`pscale auth login`.

This tool also depends on [bun](https://bun.com/) for the `serve` command to run
[poaster](https://github.com/planetscale/poaster).

# Files

onramp will store files at:

- ~/.onramp.yaml - Config
- ~/.poaster/\* - Poaster source code

## Installation

Build from source:

```bash
git clone https://github.com/planetscale/onramp
cd onramp
go install .
```

## Quick Start

```bash
onramp create              # Create and initialize a PlanetScale database
onramp serve --interactive # Start the HTTP server + user simulation
onramp serve --interactive --ssh # Start the HTTP server + user simulation, clone poaster via ssh
```

## Commands

Most of the cli is built to be interactive, however some common commands let you
run them non interactively when passed the appropriate cli flags

### `onramp create`

Create a new database on PlanetScale via an interactive wizard. Prompts for org,
name, engine, cluster size, and optionally enables Traffic Control.

```bash
onramp create
onramp create --engine postgresql --org my-org --region us-east --metal
```

### `onramp fill`

Populate the active database with dummy data. Resets the schema first unless
`--skip-init` is passed.

```bash
onramp fill --rows 1000       # Insert N rows per table
onramp fill --size 5          # Fill until ~5 GB
onramp fill --skip-init       # Skip schema reset before filling
onramp fill --max-cons 32     # Max DB connections (default: 16)
```

`--rows` and `--size` are mutually exclusive.

### `onramp serve`

Starts an HTTP server and begins hammering the database with simulated user
activity. Prints an API key to stdout on startup.

```bash
onramp serve
onramp serve --port 9090
onramp serve --interactive    # Also launch the Poaster UI (requires bun)
onramp serve --ssh            # Clone Poaster via SSH instead of HTTPS
```

| Flag            | Default | Description                                                         |
| --------------- | ------- | ------------------------------------------------------------------- |
| `--port`        | `8080`  | HTTP port to listen on                                              |
| `--max-cons`    | `8`     | Max DB connections                                                  |
| `--skip-init`   | false   | Skip schema reset on startup                                        |
| `--interactive` | false   | Also start the [Poaster](https://github.com/planetscale/poaster) UI |
| `--ssh`         | false   | Clone Poaster via SSH instead of HTTPS                              |

### `onramp clean`

Drops all tables in the active database.

```bash
onramp clean
onramp clean --force   # Skip confirmation prompt
```

### `onramp connections` (alias: `con`)

Manage saved database connections.

```bash
# Add a connection
onramp connections add --name local --dsn "mysql://user:pass@localhost:3306/mydb"
onramp connections add             # Interactive mode (PlanetScale)

# List connections (* marks the active one)
onramp connections list

# Switch active connection
onramp connections select local
onramp connections select          # Interactive picker

# Remove a connection
onramp connections remove local
```

### `onramp default-org`

Set the default PlanetScale organization (interactive).

## Configuration

onramp stores config in `~/.onramp.yaml`. The `hammer` section controls the load
simulation.

```yaml
active-connection: mydb
default-org: my-planetscale-org

connections:
  mydb:
    dsn: "mysql://user:pass@host:3306/db?tls=true"
  local-pg:
    dsn: "postgres://user:pass@localhost:5432/mydb"

hammer:
  mysql:
    new_users-per-second: 1         # Rate at which new virtual users spawn
    deleted-users-per-second: 0.5   # Rate at which virtual users are deleted
    user-flow-delay: 500ms          # Pause between flow executions per user
    max-users: 50                   # Maximum concurrent virtual users
    min-users: 0                    # Minimum concurrent virtual users
    flows:
      - name: create_post
        weight: 40                  # Probability weight for this flow before normalization
        queries:
          - "INSERT INTO posts (user_id, body) VALUES ({{.UserID}}, '{{Sentence 10}}') /*action='new_post'*/"
      - name: create_comment
        weight: 40
        queries:
          - "INSERT INTO comments (post_id, user_id, body) VALUES (1, {{.UserID}}, '{{Sentence 5}}') /*action='new_comment'*/"
      - name: delete_comment
        weight: 20
        queries:
          - "DELETE FROM comments WHERE user_id = {{.UserID}} LIMIT 1 /*action='remove_comment'*/"

  postgres:
    # Same structure — separate config per database type
    new_users-per-second: 1
    max-users: 20
    flows:
      - name: create_post
        weight: 1
        queries:
          - "INSERT INTO posts (user_id, body) VALUES ({{.UserID}}, '{{Sentence 10}}') /*action='new_post'*/"
```

Use `--config` to specify a custom config file path:

```bash
onramp --config ./my-config.yaml serve
```

## Query Templates

Queries in flows are Go templates. Available variables and functions:

| Template              | Description                                      |
| --------------------- | ------------------------------------------------ |
| `{{.UserID}}`         | Current virtual user's database ID               |
| `{{Name}}`            | Random full name                                 |
| `{{Email}}`           | Random email address                             |
| `{{Word}}`            | Random single word                               |
| `{{Sentence N}}`      | Random N-word sentence                           |
| `{{Paragraph P S W}}` | Random text (P paragraphs, S sentences, W words) |
| `{{UUID}}`            | Random UUID                                      |

String values are automatically SQL-escaped.

## HTTP API

When running `onramp serve`, the hammer is controlled via HTTP. The API key is
printed at startup and must be sent as either `X-API-Key: <key>` or
`Authorization: Bearer <key>`.

| Method | Path             | Description                          |
| ------ | ---------------- | ------------------------------------ |
| GET    | `/users`         | Get current virtual user config      |
| POST   | `/users`         | Update virtual user config           |
| GET    | `/users/current` | Get currently active simulated users |
| GET    | `/users/flows`   | Get current flows                    |
| POST   | `/users/flows`   | Replace flows                        |
| POST   | `/hammer/start`  | Start the hammer                     |
| POST   | `/hammer/stop`   | Stop the hammer                      |

```bash
# Update concurrency settings
curl -X POST http://localhost:8080/users \
  -H "X-API-Key: <key>" \
  -H "Content-Type: application/json" \
  -d '{"max_users": 100, "min_users": 10, "new_users_per_second": 2, "deleted_users_per_second": 1, "user_flow_delay": "200ms"}'

# Start the hammer
curl -X POST http://localhost:8080/hammer/start -H "X-API-Key: <key>"
```

## Schema

The tables created on initialization:

```
users             (id, name, email, created_at)
posts             (id, user_id, body, created_at)
comments          (id, post_id, user_id, body, created_at)
tags              (id, name)
post_tags         (post_id, tag_id)
```

PostgreSQL databases also get:

```
activity          (id, user_id, action, target_type, target_id, metadata, created_at)
report_snapshots  (id, name)
```

Schema statements can be overridden in `~/.onramp.yaml` under `schema.mysql` or
`schema.postgres`.
