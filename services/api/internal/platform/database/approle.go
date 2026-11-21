package database

// Run the request path on a non-superuser database role (ADR-036 T2, SPA-9343).
//
// Before this file every pool this process opened — the request path, the
// workers, and the SQL that WASM plugins run through paca.db_query /
// paca.db_exec — connected as the role that OWNS the tenant databases: on
// staging that is `paca`, a SUPERUSER with BYPASSRLS. Any statement that path
// could be talked into running ran with the power to read every database on the
// cluster.
//
// With DB_APP_ROLE set (e.g. `paca_app`) each tenant gets two pools:
//
//   - the OWNER pool (the configured DSN, as before) creates the database when
//     it is missing, runs the schema migrations and the plugin migrations
//     (CREATE SCHEMA / CREATE TABLE when a plugin is installed), and nothing
//     else. It is kept small;
//   - the APP pool connects as DB_APP_ROLE: NOSUPERUSER, NOBYPASSRLS, no
//     CREATEDB / CREATEROLE, a member of no other role, with SELECT, INSERT,
//     UPDATE, DELETE on the tables, and USAGE/SELECT on the sequences, of every
//     schema the owner has created — public and every plugin schema — and the
//     same on whatever the owner creates later (default privileges). Every
//     repository, worker and the plugin runtime use this pool.
//
// The app role's password is HMAC-SHA256(owner password, role) — the same
// derivation as app_role.py in Galaxy-AI-Crawler / Galaxy-AI-Hackathon — so no
// new secret has to be created, stored or typed, and the notify bridges can
// derive the same DSN from the PACA_DATABASE_URL they already hold. It is set
// as a SCRAM verifier, so the clear text never travels in SQL, and only when
// the stored verifier does not already match.

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// AppRoleEnv names the environment variable that switches the feature on.
const AppRoleEnv = "DB_APP_ROLE"

const kdfLabel = "galaxy-db-app-role/v1/"

var roleNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// ValidateAppRole reports whether role can be used as DB_APP_ROLE. The empty
// string is valid: it means the feature is off.
func ValidateAppRole(role string) error {
	if role == "" || roleNameRE.MatchString(role) {
		return nil
	}
	return fmt.Errorf("%s=%q is not a plain lower-case role name", AppRoleEnv, role)
}

func dsnCredentials(dsn string) (*url.URL, string, string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, "", "", fmt.Errorf("dsn is not a URL: %w", err)
	}
	if !strings.HasPrefix(u.Scheme, "postgres") {
		return nil, "", "", fmt.Errorf("dsn is not a postgres URL (scheme %q)", u.Scheme)
	}
	if u.User == nil {
		return nil, "", "", errors.New("dsn carries no user/password")
	}
	pw, ok := u.User.Password()
	if !ok || pw == "" {
		return nil, "", "", errors.New("dsn carries no password to derive the app role's from")
	}
	return u, u.User.Username(), pw, nil
}

// DeriveAppPassword returns the app role's password for an owner password.
func DeriveAppPassword(ownerPassword, role string) string {
	m := hmac.New(sha256.New, []byte(ownerPassword))
	m.Write([]byte(kdfLabel + role))
	return hex.EncodeToString(m.Sum(nil))
}

// AppRoleDSN returns ownerDSN with the owner's credentials replaced by role's.
func AppRoleDSN(ownerDSN, role string) (string, error) {
	u, _, pw, err := dsnCredentials(ownerDSN)
	if err != nil {
		return "", err
	}
	app := *u
	app.User = url.UserPassword(role, DeriveAppPassword(pw, role))
	return app.String(), nil
}

// ScramVerifier returns the SCRAM-SHA-256 verifier PostgreSQL stores for
// password (RFC 5802 / 7677).
func ScramVerifier(password string, salt []byte, iterations int) (string, error) {
	stored, server, err := scramKeys(password, salt, iterations)
	if err != nil {
		return "", err
	}
	b := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", iterations, b(salt), b(stored), b(server)), nil
}

func scramKeys(password string, salt []byte, iterations int) ([]byte, []byte, error) {
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return nil, nil, err
	}
	mac := func(key []byte, msg string) []byte {
		m := hmac.New(sha256.New, key)
		m.Write([]byte(msg))
		return m.Sum(nil)
	}
	clientKey := mac(salted, "Client Key")
	stored := sha256.Sum256(clientKey)
	return stored[:], mac(salted, "Server Key"), nil
}

var verifierRE = regexp.MustCompile(`^SCRAM-SHA-256\$(\d+):([^$]+)\$([^:]+):(.+)$`)

// VerifierMatches reports whether stored is a SCRAM verifier of password.
func VerifierMatches(stored, password string) bool {
	m := verifierRE.FindStringSubmatch(stored)
	if m == nil {
		return false
	}
	iterations, err := strconv.Atoi(m[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err1 := base64.StdEncoding.DecodeString(m[2])
	storedKey, err2 := base64.StdEncoding.DecodeString(m[3])
	serverKey, err3 := base64.StdEncoding.DecodeString(m[4])
	if err1 != nil || err2 != nil || err3 != nil {
		return false
	}
	wantStored, wantServer, err := scramKeys(password, salt, iterations)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(wantStored, storedKey) == 1 &&
		subtle.ConstantTimeCompare(wantServer, serverKey) == 1
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

// ProvisionAppRole makes role a DML-only login on the database owner reaches,
// connecting as the owner. ownerDSN is only read for the owner's name and
// password (to derive role's). Safe to run on every start.
func ProvisionAppRole(ctx context.Context, owner *sql.DB, ownerDSN, role string) error {
	if err := ValidateAppRole(role); err != nil || role == "" {
		if err == nil {
			err = errors.New("no app role given")
		}
		return err
	}
	_, ownerName, ownerPW, err := dsnCredentials(ownerDSN)
	if err != nil {
		return err
	}
	if ownerName == role {
		return fmt.Errorf("%s=%q is the owner role itself", AppRoleEnv, role)
	}
	password := DeriveAppPassword(ownerPW, role)
	rid := quoteIdent(role)
	oid := quoteIdent(ownerName)

	exec := func(q string) error {
		if _, err := owner.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("app role: %s: %w", strings.SplitN(q, " TO ", 2)[0], err)
		}
		return nil
	}

	var exists bool
	if err := owner.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, role).Scan(&exists); err != nil {
		return fmt.Errorf("app role: look up %s: %w", role, err)
	}
	if !exists {
		if err := exec(`CREATE ROLE ` + rid + ` LOGIN`); err != nil {
			return err
		}
	}
	if err := exec(`ALTER ROLE ` + rid + ` WITH LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT`); err != nil {
		return err
	}
	var stored sql.NullString
	if err := owner.QueryRowContext(ctx,
		`SELECT rolpassword FROM pg_authid WHERE rolname = $1`, role).Scan(&stored); err != nil {
		stored = sql.NullString{} // a non-superuser owner cannot read pg_authid: just set it
	}
	if !VerifierMatches(stored.String, password) {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return fmt.Errorf("app role: salt: %w", err)
		}
		v, err := ScramVerifier(password, salt, 4096)
		if err != nil {
			return fmt.Errorf("app role: verifier: %w", err)
		}
		// A verifier, not the password, is sent; the error never echoes it.
		if _, err := owner.ExecContext(ctx, `ALTER ROLE `+rid+` PASSWORD `+quoteLiteral(v)); err != nil {
			return fmt.Errorf("app role: set password of %s: %w", role, err)
		}
	}

	var parents []string
	rows, err := owner.QueryContext(ctx, `
		SELECT m.rolname FROM pg_auth_members a
		JOIN pg_roles r ON r.oid = a.member JOIN pg_roles m ON m.oid = a.roleid
		WHERE r.rolname = $1`, role)
	if err != nil {
		return fmt.Errorf("app role: memberships: %w", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			_ = rows.Close()
			return err
		}
		parents = append(parents, p)
	}
	_ = rows.Close()
	if len(parents) > 0 {
		return fmt.Errorf("app role: %s is a member of %v — refusing to run on it", role, parents)
	}
	var super, bypass, createdb, createrole bool
	if err := owner.QueryRowContext(ctx,
		`SELECT rolsuper, rolbypassrls, rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname = $1`,
		role).Scan(&super, &bypass, &createdb, &createrole); err != nil {
		return fmt.Errorf("app role: attributes: %w", err)
	}
	if super || bypass || createdb || createrole {
		return fmt.Errorf("app role: %s still has SUPERUSER/BYPASSRLS/CREATEDB/CREATEROLE", role)
	}

	var dbname string
	if err := owner.QueryRowContext(ctx, `SELECT current_database()`).Scan(&dbname); err != nil {
		return err
	}
	if err := exec(`GRANT CONNECT ON DATABASE ` + quoteIdent(dbname) + ` TO ` + rid); err != nil {
		return err
	}

	// Every schema the owner holds: public (owned by pg_database_owner on
	// PG15+) and one per installed plugin.
	schemas, err := ownedSchemas(ctx, owner, ownerName)
	if err != nil {
		return err
	}
	for _, s := range schemas {
		sid := quoteIdent(s)
		for _, q := range []string{
			`GRANT USAGE ON SCHEMA ` + sid + ` TO ` + rid,
			`REVOKE CREATE ON SCHEMA ` + sid + ` FROM ` + rid,
			`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ` + sid + ` TO ` + rid,
			`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ` + sid + ` TO ` + rid,
		} {
			if err := exec(q); err != nil {
				return err
			}
		}
		// A plugin's migration ledger is the owner's business only.
		var ledger bool
		if err := owner.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`,
			sid+`.plugin_schema_migrations`).Scan(&ledger); err != nil {
			return err
		}
		if ledger {
			if err := exec(`REVOKE ALL ON TABLE ` + sid + `.plugin_schema_migrations FROM ` + rid); err != nil {
				return err
			}
		}
	}
	// What the owner creates later — a migration's new table, a plugin
	// installed at run time — is usable by the app role without a restart.
	for _, q := range []string{
		`ALTER DEFAULT PRIVILEGES FOR ROLE ` + oid + ` GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ` + rid,
		`ALTER DEFAULT PRIVILEGES FOR ROLE ` + oid + ` GRANT USAGE, SELECT ON SEQUENCES TO ` + rid,
		`ALTER DEFAULT PRIVILEGES FOR ROLE ` + oid + ` GRANT USAGE ON SCHEMAS TO ` + rid,
	} {
		if err := exec(q); err != nil {
			return err
		}
	}
	return nil
}

func ownedSchemas(ctx context.Context, owner *sql.DB, ownerName string) ([]string, error) {
	rows, err := owner.QueryContext(ctx, `
		SELECT nspname FROM pg_namespace
		WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema'
		  AND (nspname = 'public' OR pg_get_userbyid(nspowner) = $1)
		ORDER BY nspname`, ownerName)
	if err != nil {
		return nil, fmt.Errorf("app role: schemas: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
