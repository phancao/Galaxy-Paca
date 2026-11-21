package database_test

// Unit tests need nothing. The real-PostgreSQL tests need a superuser DSN to a
// throw-away server in PACA_TEST_PG_ADMIN_DSN (any database; each test creates
// its own) and are skipped, loudly, without it. They apply THIS service's real
// migrations as the owner and check with real SQL what the app role can and
// cannot do.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Paca-AI/api/internal/platform/database"
	"github.com/Paca-AI/api/migrations"
)

// Reference values computed by app_role.py (Galaxy-AI-Crawler #55): the notify
// bridges and the Python services must derive the very same password.
const (
	refOwnerPW  = "o:w/n@r%pw"
	refPassword = "bf5faf1d3fa88248c95361ca56038a180ca8d5a539c57f63ebfb90bd9f2823eb"
	refVerifier = "SCRAM-SHA-256$4096:AAECAwQFBgcICQoLDA0ODw==$THoPhoTAuqyoQsK4dUHncUzgfD8fdmhsgKZhWVqNP5U=:7YiHMMi2OcXGRogub03Ek06JRZ9bkhTOdCzHa5iPLiQ="
)

func TestDeriveAppPasswordMatchesThePythonServices(t *testing.T) {
	if got := database.DeriveAppPassword(refOwnerPW, "paca_app"); got != refPassword {
		t.Fatalf("derived %s, want %s", got, refPassword)
	}
	if database.DeriveAppPassword(refOwnerPW, "other_app") == refPassword {
		t.Fatal("password must depend on the role")
	}
	if database.DeriveAppPassword("x", "paca_app") == refPassword {
		t.Fatal("password must depend on the owner password")
	}
}

func TestAppRoleDSNSwapsOnlyTheCredentials(t *testing.T) {
	owner := "postgres://paca:" + url.QueryEscape(refOwnerPW) + "@postgres:5432/paca_kiemthu?sslmode=disable"
	got, err := database.AppRoleDSN(owner, "paca_app")
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://paca_app:" + refPassword + "@postgres:5432/paca_kiemthu?sslmode=disable"
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	if strings.Contains(got, refOwnerPW) || strings.Contains(got, url.QueryEscape(refOwnerPW)) {
		t.Fatal("owner password leaked into the app DSN")
	}
}

func TestAppRoleDSNRefusesWhatItCannotDeriveFrom(t *testing.T) {
	for _, bad := range []string{
		"postgres://postgres:5432/x",
		"postgres://owner@postgres:5432/x",
		"mysql://o:p@db/x",
		"::not a url",
	} {
		if _, err := database.AppRoleDSN(bad, "paca_app"); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestScramVerifierMatchesPython(t *testing.T) {
	salt := make([]byte, 16)
	for i := range salt {
		salt[i] = byte(i)
	}
	v, err := database.ScramVerifier("secret", salt, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if v != refVerifier {
		t.Fatalf("got %s\nwant %s", v, refVerifier)
	}
	if !database.VerifierMatches(v, "secret") || database.VerifierMatches(v, "Secret") ||
		database.VerifierMatches("", "secret") || database.VerifierMatches("md5abc", "secret") {
		t.Fatal("VerifierMatches is wrong")
	}
}

func TestValidateAppRole(t *testing.T) {
	for _, ok := range []string{"", "paca_app", "_x", "a1"} {
		if err := database.ValidateAppRole(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"Paca", "1a", "a b", `a"`, "a;drop", strings.Repeat("a", 64)} {
		if err := database.ValidateAppRole(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

// ── real PostgreSQL ──────────────────────────────────────────────────────────

const appRole = "paca_app_test"

func adminDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("PACA_TEST_PG_ADMIN_DSN")
	if dsn == "" {
		t.Skip("PACA_TEST_PG_ADMIN_DSN not set — real-SQL app-role tests not run")
	}
	return dsn
}

func withDB(dsn, name string) string {
	u, _ := url.Parse(dsn)
	u.Path = "/" + name
	return u.String()
}

// tenantDB creates a fresh database, applies the real migrations as the owner
// and returns the owner pool and DSN.
func tenantDB(t *testing.T, name string) (*sql.DB, string) {
	t.Helper()
	admin := adminDSN(t)
	a, err := sql.Open("pgx", admin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	ctx := context.Background()
	_, _ = a.ExecContext(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
	if _, err := a.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatal(err)
	}
	dsn := withDB(admin, name)
	owner, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	if err := database.RunMigrationsFS(owner, migrations.FS); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return owner, dsn
}

func appPool(t *testing.T, ownerDSN, role string) *sql.DB {
	t.Helper()
	dsn, err := database.AppRoleDSN(ownerDSN, role)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func mustDeny(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), q)
	if sqlState(err) != "42501" {
		t.Errorf("%s: want insufficient_privilege, got %v", q, err)
	}
}

func appTables(t *testing.T, owner *sql.DB) []string {
	t.Helper()
	rows, err := owner.QueryContext(context.Background(),
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func TestAppRoleIsDMLOnlyOnRealPostgres(t *testing.T) {
	ctx := context.Background()
	owner, dsn := tenantDB(t, "paca_approle_t1")
	// A plugin installed before the switch: its schema already exists.
	for _, q := range []string{
		`CREATE SCHEMA plugin_before`,
		`CREATE TABLE plugin_before.plugin_kv (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE plugin_before.plugin_schema_migrations (filename TEXT PRIMARY KEY)`,
	} {
		if _, err := owner.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.ProvisionAppRole(ctx, owner, dsn, appRole); err != nil {
		t.Fatal(err)
	}

	var super, bypass, createdb, createrole bool
	if err := owner.QueryRowContext(ctx, `SELECT rolsuper, rolbypassrls, rolcreatedb, rolcreaterole
		FROM pg_roles WHERE rolname = $1`, appRole).Scan(&super, &bypass, &createdb, &createrole); err != nil {
		t.Fatal(err)
	}
	if super || bypass || createdb || createrole {
		t.Fatalf("app role attributes: super=%v bypass=%v createdb=%v createrole=%v", super, bypass, createdb, createrole)
	}

	app := appPool(t, dsn, appRole)
	var who string
	if err := app.QueryRowContext(ctx, `SELECT current_user`).Scan(&who); err != nil || who != appRole {
		t.Fatalf("current_user = %q (%v)", who, err)
	}
	tables := appTables(t, owner)
	if len(tables) < 10 {
		t.Fatalf("migrations created only %d tables", len(tables))
	}
	for _, tb := range tables {
		for _, q := range []string{
			`SELECT count(*) FROM "` + tb + `"`,
			`DELETE FROM "` + tb + `" WHERE false`,
		} {
			if _, err := app.ExecContext(ctx, q); err != nil {
				t.Errorf("%s: %v", q, err)
			}
		}
	}
	// The plugin's own table works the way paca.db_exec uses it…
	tx, err := app.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path TO plugin_before,public`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO plugin_kv (key, value) VALUES ('k', 'v')`); err != nil {
		t.Fatalf("plugin_kv insert: %v", err)
	}
	_ = tx.Rollback()
	// …its migration ledger does not.
	mustDeny(t, app, `SELECT * FROM plugin_before.plugin_schema_migrations`)

	// No DDL, no cluster-level power.
	mustDeny(t, app, `CREATE TABLE sneaky (x int)`)
	mustDeny(t, app, `CREATE TABLE plugin_before.sneaky (x int)`)
	mustDeny(t, app, `ALTER TABLE "`+tables[0]+`" ADD COLUMN sneaky int`)
	mustDeny(t, app, `DROP TABLE "`+tables[0]+`"`)
	mustDeny(t, app, `TRUNCATE "`+tables[0]+`"`)
	mustDeny(t, app, `CREATE SCHEMA sneaky`)
	mustDeny(t, app, `CREATE DATABASE sneaky`)
	mustDeny(t, app, `CREATE ROLE sneaky`)
	mustDeny(t, app, `SELECT rolpassword FROM pg_authid`)
	mustDeny(t, app, `ALTER ROLE `+appRole+` BYPASSRLS`)

	// A plugin installed AFTER the switch (the owner runs its migration) is
	// usable at once, through default privileges, without a restart.
	for _, q := range []string{
		`CREATE SCHEMA plugin_after`,
		`CREATE TABLE plugin_after.plugin_kv (id serial PRIMARY KEY, value TEXT)`,
	} {
		if _, err := owner.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	var id int
	if err := app.QueryRowContext(ctx,
		`INSERT INTO plugin_after.plugin_kv (value) VALUES ('x') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("new plugin table: %v", err)
	}
}

func TestProvisionAppRoleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	owner, dsn := tenantDB(t, "paca_approle_t2")
	if err := database.ProvisionAppRole(ctx, owner, dsn, appRole); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		var v string
		if err := owner.QueryRowContext(ctx, `SELECT rolpassword FROM pg_authid WHERE rolname = $1`, appRole).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := read()
	if err := database.ProvisionAppRole(ctx, owner, dsn, appRole); err != nil {
		t.Fatal(err)
	}
	if read() != before {
		t.Fatal("a second run changed the stored verifier")
	}
}

func TestProvisionAppRoleDemotesAndRefuses(t *testing.T) {
	ctx := context.Background()
	owner, dsn := tenantDB(t, "paca_approle_t3")
	u, _ := url.Parse(dsn)
	for _, q := range []string{
		`DROP ROLE IF EXISTS was_super_t`, `DROP ROLE IF EXISTS member_t`,
		`CREATE ROLE was_super_t LOGIN SUPERUSER BYPASSRLS CREATEDB CREATEROLE`,
		`CREATE ROLE member_t LOGIN`,
		fmt.Sprintf(`GRANT %q TO member_t`, u.User.Username()),
	} {
		if _, err := owner.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.ProvisionAppRole(ctx, owner, dsn, "was_super_t"); err != nil {
		t.Fatal(err)
	}
	var super, bypass bool
	if err := owner.QueryRowContext(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname='was_super_t'`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if super || bypass {
		t.Fatal("an existing superuser was not demoted")
	}
	if err := database.ProvisionAppRole(ctx, owner, dsn, "member_t"); err == nil || !strings.Contains(err.Error(), "member of") {
		t.Fatalf("a role inheriting from another must be refused, got %v", err)
	}
	if err := database.ProvisionAppRole(ctx, owner, dsn, u.User.Username()); err == nil || !strings.Contains(err.Error(), "owner role itself") {
		t.Fatalf("the owner itself must be refused, got %v", err)
	}
}

func TestOpenAsAppRoleNeverCreatesADatabase(t *testing.T) {
	ctx := context.Background()
	owner, dsn := tenantDB(t, "paca_approle_t4")
	if err := database.ProvisionAppRole(ctx, owner, dsn, appRole); err != nil {
		t.Fatal(err)
	}
	missing, err := database.AppRoleDSN(withDB(dsn, "paca_approle_missing"), appRole)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := database.Open(database.Config{DSN: missing}, log); err == nil {
		t.Fatal("the app role must not be able to create a database")
	}
}
