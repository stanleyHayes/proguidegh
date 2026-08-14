// Package migrations is a tiny, dependency-free versioned SQL migration
// runner. Migration files are embedded and named NNNN_name.up.sql /
// NNNN_name.down.sql; applied versions are recorded in schema_migrations.
package migrations

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var files embed.FS

// Migration is a single versioned up/down pair.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// Status pairs a migration with whether it has been applied.
type Status struct {
	Migration Migration
	Applied   bool
}

// Load parses the embedded migration files, sorted by version.
func Load() ([]Migration, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("migrations: read embedded dir: %w", err)
	}

	ups := map[int]Migration{}
	downs := map[int]string{}
	for _, e := range entries {
		version, name, dir, err := parseFilename(e.Name())
		if err != nil {
			return nil, err
		}
		body, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("migrations: read %s: %w", e.Name(), err)
		}
		switch dir {
		case "up":
			if _, dup := ups[version]; dup {
				return nil, fmt.Errorf("migrations: duplicate up version %04d", version)
			}
			ups[version] = Migration{Version: version, Name: name, Up: string(body)}
		case "down":
			downs[version] = string(body)
		}
	}

	out := make([]Migration, 0, len(ups))
	for version, m := range ups {
		down, ok := downs[version]
		if !ok {
			return nil, fmt.Errorf("migrations: version %04d has an up file but no down file", version)
		}
		m.Down = down
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// parseFilename splits "0001_init.up.sql" into (1, "init", "up").
func parseFilename(name string) (int, string, string, error) {
	trimmed := strings.TrimSuffix(name, ".sql")
	if trimmed == name {
		return 0, "", "", fmt.Errorf("migrations: %q is not a .sql file", name)
	}
	parts := strings.SplitN(trimmed, "_", 2)
	if len(parts) != 2 {
		return 0, "", "", fmt.Errorf("migrations: bad filename %q, want NNNN_name.{up,down}.sql", name)
	}
	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", "", fmt.Errorf("migrations: bad version in %q: %w", name, err)
	}
	var base, dir string
	switch {
	case strings.HasSuffix(parts[1], ".up"):
		base, dir = strings.TrimSuffix(parts[1], ".up"), "up"
	case strings.HasSuffix(parts[1], ".down"):
		base, dir = strings.TrimSuffix(parts[1], ".down"), "down"
	default:
		return 0, "", "", fmt.Errorf("migrations: bad direction in %q, want .up or .down", name)
	}
	return version, base, dir, nil
}

// EnsureTable creates the schema_migrations bookkeeping table.
func EnsureTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    integer PRIMARY KEY,
			name       text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("migrations: ensure schema_migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("migrations: list applied: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("migrations: scan applied: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Up applies all pending migrations in version order, each in its own
// transaction. Returns the versions applied by this call.
func Up(ctx context.Context, pool *pgxpool.Pool) ([]int, error) {
	migrations, err := Load()
	if err != nil {
		return nil, err
	}
	if err := EnsureTable(ctx, pool); err != nil {
		return nil, err
	}
	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}

	var done []int
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if err := applyUp(ctx, pool, m); err != nil {
			return done, err
		}
		done = append(done, m.Version)
	}
	return done, nil
}

func applyUp(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrations: begin up %04d: %w", m.Version, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	if _, err := tx.Exec(ctx, m.Up); err != nil {
		return fmt.Errorf("migrations: apply %04d_%s.up.sql: %w", m.Version, m.Name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name); err != nil {
		return fmt.Errorf("migrations: record %04d: %w", m.Version, err)
	}
	return tx.Commit(ctx)
}

// Down rolls back the latest applied migration (or all of them when
// all=true), each in its own transaction. Returns the versions reverted.
func Down(ctx context.Context, pool *pgxpool.Pool, all bool) ([]int, error) {
	if err := EnsureTable(ctx, pool); err != nil {
		return nil, err
	}
	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}

	migrations, err := Load()
	if err != nil {
		return nil, err
	}

	var done []int
	for i := len(migrations) - 1; i >= 0; i-- {
		m := migrations[i]
		if !applied[m.Version] {
			continue
		}
		if err := applyDown(ctx, pool, m); err != nil {
			return done, err
		}
		done = append(done, m.Version)
		if !all {
			break
		}
	}
	return done, nil
}

func applyDown(ctx context.Context, pool *pgxpool.Pool, m Migration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrations: begin down %04d: %w", m.Version, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	if _, err := tx.Exec(ctx, m.Down); err != nil {
		return fmt.Errorf("migrations: apply %04d_%s.down.sql: %w", m.Version, m.Name, err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM schema_migrations WHERE version = $1`, m.Version); err != nil {
		return fmt.Errorf("migrations: unrecord %04d: %w", m.Version, err)
	}
	return tx.Commit(ctx)
}

// Statuses reports every known migration with its applied flag.
func Statuses(ctx context.Context, pool *pgxpool.Pool) ([]Status, error) {
	if err := EnsureTable(ctx, pool); err != nil {
		return nil, err
	}
	applied, err := appliedVersions(ctx, pool)
	if err != nil {
		return nil, err
	}
	migrations, err := Load()
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(migrations))
	for _, m := range migrations {
		out = append(out, Status{Migration: m, Applied: applied[m.Version]})
	}
	return out, nil
}
