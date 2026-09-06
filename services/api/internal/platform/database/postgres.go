// Package database provides PostgreSQL connectivity via sqlx.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // register "pgx" driver
	"github.com/jmoiron/sqlx"
)

// Config holds the settings required to open a database connection.
type Config struct {
	// DSN is the PostgreSQL connection string.
	DSN string
}

// Open establishes a sqlx PostgreSQL connection using the settings in cfg.
func Open(cfg Config, log *slog.Logger) (*sqlx.DB, error) {
	db, err := sqlx.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(context.Background()); err != nil {
		// A tenant added to the configuration has no database yet. Creating
		// it here is what makes "add a tenant" a one-line change instead of
		// a runbook: the schema arrives right after, because migrations are
		// idempotent and run on every startup anyway.
		//
		// Narrow on purpose. Only the "does not exist" code (3D000) is
		// treated this way; every other connection failure — wrong password,
		// unreachable host, exhausted pool — is returned untouched, because
		// creating a database is never the fix for those and trying would
		// only bury the real error under a second one.
		if !isUndefinedDatabase(err) {
			return nil, fmt.Errorf("database: ping: %w", err)
		}
		_ = db.Close()
		name, createErr := createDatabase(cfg.DSN, log)
		if createErr != nil {
			return nil, fmt.Errorf("database: ping: %w (and could not create it: %v)", err, createErr)
		}
		log.Info("database created", "database", name)
		db, err = sqlx.Open("pgx", cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("database: reopen: %w", err)
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(30 * time.Minute)
		if err := db.PingContext(context.Background()); err != nil {
			return nil, fmt.Errorf("database: ping after create: %w", err)
		}
	}

	log.Info("database connected")
	return db, nil
}

// isUndefinedDatabase reports whether err is Postgres 3D000, the code for a
// database that does not exist.
func isUndefinedDatabase(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "3D000"
	}
	// pgx wraps the connect error; fall back to the message when it does.
	return strings.Contains(strings.ToLower(err.Error()), "does not exist")
}

// createDatabase connects to the `postgres` maintenance database on the same
// server and creates the one named in dsn. Returns its name.
//
// CREATE DATABASE cannot run inside a transaction, and the name comes from
// our own configuration rather than from a request — but it is still quoted,
// because "it cannot be hostile today" is how an injection gets in tomorrow.
func createDatabase(dsn string, log *slog.Logger) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("dsn is not a URL: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("dsn names no database")
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z') && !(c >= '0' && c <= '9') && c != '_' {
			return "", fmt.Errorf("refusing to create database %q: name must be lowercase letters, digits or underscore", name)
		}
	}

	admin := *u
	admin.Path = "/postgres"
	conn, err := sqlx.Open("pgx", admin.String())
	if err != nil {
		return "", fmt.Errorf("open maintenance connection: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.PingContext(context.Background()); err != nil {
		return "", fmt.Errorf("ping maintenance connection: %w", err)
	}

	log.Info("database missing — creating it", "database", name)
	if _, err := conn.ExecContext(context.Background(), `CREATE DATABASE "`+name+`"`); err != nil {
		// Another replica starting at the same moment may have won the race.
		// That is success, not failure: the database exists either way.
		if strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return name, nil
		}
		return "", fmt.Errorf("create database %q: %w", name, err)
	}
	return name, nil
}
