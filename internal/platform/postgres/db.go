// Package postgres is the PostgreSQL adapter: connection pooling and schema
// migration. It depends on the standard library and pgx only. The migration
// runner is intentionally small and auditable (see docs/adr/0004); a larger
// framework can replace it later behind the same Connect/Migrate surface.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jeston10/pipeline-sherlock/migrations"
)

// DB wraps a pgx connection pool. It is safe for concurrent use.
type DB struct {
	Pool *pgxpool.Pool
}

// pingTimeout bounds the connectivity check performed at connect time and by
// the readiness endpoint.
const pingTimeout = 3 * time.Second

// Connect opens a connection pool to the given PostgreSQL URL and verifies
// connectivity with a bounded ping. The caller owns the returned *DB and must
// call Close when done.
func Connect(ctx context.Context, databaseURL string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Ping verifies the database is reachable, bounded by a short timeout. It
// satisfies the readiness-check contract consumed by the HTTP layer.
func (db *DB) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.Pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("database ping: %w", err)
	}
	return nil
}

// Close releases all pooled connections.
func (db *DB) Close() {
	if db != nil && db.Pool != nil {
		db.Pool.Close()
	}
}

// migration is one versioned change read from the embedded filesystem.
type migration struct {
	version int64
	name    string
	up      string
	down    string
}

// Migrate applies all pending up-migrations in version order. It is safe to
// run repeatedly: already-applied versions are skipped. Each migration file is
// executed as a single batch inside its own transaction, and the applied
// version is recorded in schema_migrations within that same transaction.
func Migrate(ctx context.Context, db *DB) error {
	if err := ensureMigrationsTable(ctx, db.Pool); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db.Pool)
	if err != nil {
		return err
	}
	all, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range all {
		if applied[m.version] {
			continue
		}
		if err := applyUp(ctx, db.Pool, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}
	return nil
}

// Rollback reverts the most recently applied migration using its down script.
// It is used by `make migrate-down` and by tests; it is never run automatically
// at startup.
func Rollback(ctx context.Context, db *DB) error {
	if err := ensureMigrationsTable(ctx, db.Pool); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, db.Pool)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		return nil
	}
	all, err := loadMigrations()
	if err != nil {
		return err
	}

	// Find the highest applied version.
	var latest *migration
	for i := range all {
		if applied[all[i].version] {
			latest = &all[i]
		}
	}
	if latest == nil {
		return nil
	}
	if strings.TrimSpace(latest.down) == "" {
		return fmt.Errorf("migration %d (%s) has no down script", latest.version, latest.name)
	}
	return applyDown(ctx, db.Pool, *latest)
}

func ensureMigrationsTable(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    bigint PRIMARY KEY,
    name       text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);`
	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	return nil
}

func appliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int64]bool, error) {
	rows, err := pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("query applied versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan applied version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied versions: %w", err)
	}
	return applied, nil
}

func applyUp(ctx context.Context, pool *pgxpool.Pool, m migration) error {
	return withTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.up); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`,
			m.version, m.name)
		return err
	})
}

func applyDown(ctx context.Context, pool *pgxpool.Pool, m migration) error {
	return withTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.down); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.version)
		return err
	})
}

func withTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			// Roll back on failure; ignore rollback error in favour of the
			// original cause but join it if present.
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// loadMigrations reads and pairs up/down SQL files from the embedded FS,
// returning them sorted by ascending version.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	byVersion := make(map[int64]*migration)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version, name, direction, perr := parseMigrationFilename(e.Name())
		if perr != nil {
			return nil, perr
		}
		body, rerr := fs.ReadFile(migrations.FS, e.Name())
		if rerr != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), rerr)
		}
		m := byVersion[version]
		if m == nil {
			m = &migration{version: version, name: name}
			byVersion[version] = m
		}
		switch direction {
		case "up":
			m.up = string(body)
		case "down":
			m.down = string(body)
		}
	}

	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.up == "" {
			return nil, fmt.Errorf("migration %d (%s) is missing an up script", m.version, m.name)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// parseMigrationFilename parses "<version>_<name>.<up|down>.sql".
func parseMigrationFilename(filename string) (version int64, name, direction string, err error) {
	base := strings.TrimSuffix(filename, ".sql")
	dot := strings.LastIndex(base, ".")
	if dot < 0 {
		return 0, "", "", fmt.Errorf("migration %q missing .up/.down suffix", filename)
	}
	direction = base[dot+1:]
	if direction != "up" && direction != "down" {
		return 0, "", "", fmt.Errorf("migration %q has unknown direction %q", filename, direction)
	}
	rest := base[:dot]
	us := strings.Index(rest, "_")
	if us <= 0 {
		return 0, "", "", fmt.Errorf("migration %q missing version prefix", filename)
	}
	version, err = strconv.ParseInt(rest[:us], 10, 64)
	if err != nil {
		return 0, "", "", fmt.Errorf("migration %q has non-numeric version: %w", filename, err)
	}
	name = rest[us+1:]
	return version, name, direction, nil
}
