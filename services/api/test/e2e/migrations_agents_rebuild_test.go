package e2e_test

// SPA-8852: staging's tenant "galaxy" crash-looped because paca.agents had
// reached Postgres' 1600-attnum limit (1582 dropped columns), left behind by
// migrations that re-added and re-dropped columns on every boot. #62 stopped
// the churn; 000033 rebuilds agents to give the attnums back.
//
// These tests run the real embedded migration set against a real Postgres:
//   - in CI through the e2e testcontainers Postgres (PACA_E2E=1), or
//   - against any Postgres you point PACA_MIGRATIONS_PG_DSN at (an admin DSN
//     that may CREATE DATABASE), which is how they run without Docker.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/Paca-AI/api/internal/platform/database"
	"github.com/Paca-AI/api/migrations"
)

var migrationDBSeq atomic.Int64

// newMigrationDB returns a connection to a fresh, empty database.
func newMigrationDB(t *testing.T) *sqlx.DB {
	t.Helper()
	adminDSN := os.Getenv("PACA_MIGRATIONS_PG_DSN")
	if adminDSN == "" {
		if os.Getenv("PACA_E2E") != "1" {
			t.Skip("set PACA_E2E=1 (Docker) or PACA_MIGRATIONS_PG_DSN to run migration tests against Postgres")
		}
		checkDockerAvailable(t)
		adminDSN = sharedPGDSN
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	admin, err := database.Open(database.Config{DSN: adminDSN}, log)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	name := fmt.Sprintf("mig_spa8852_%d_%d", os.Getpid(), migrationDBSeq.Add(1))
	if _, err := admin.ExecContext(context.Background(), fmt.Sprintf(`CREATE DATABASE %q`, name)); err != nil {
		_ = admin.Close()
		t.Fatalf("create database: %v", err)
	}
	u, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	db, err := database.Open(database.Config{DSN: u.String()}, log)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`, name)
		_, _ = admin.ExecContext(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q`, name))
		_ = admin.Close()
	})
	return db
}

func migrate(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if err := database.RunMigrationsFS(db.DB, migrations.FS); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

// attnums returns, for every user table, its highest attnum and dropped count.
func attnums(t *testing.T, db *sqlx.DB) map[string][2]int {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT c.oid::regclass::text, max(a.attnum), count(*) FILTER (WHERE a.attisdropped)
		  FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind = 'r' AND a.attnum > 0 AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		 GROUP BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][2]int{}
	for rows.Next() {
		var name string
		var mx, dropped int
		if err := rows.Scan(&name, &mx, &dropped); err != nil {
			t.Fatal(err)
		}
		out[name] = [2]int{mx, dropped}
	}
	return out
}

// agentsShape is everything about agents a rebuild must keep: live columns
// (name, type, nullability, default, in order), constraints, indexes, foreign
// keys pointing at it, owner, grants, RLS flags.
func agentsShape(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	var parts []string
	q := func(label, query string) {
		var rows []string
		if err := db.Select(&rows, query); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		sort.Strings(rows)
		parts = append(parts, label+":\n  "+strings.Join(rows, "\n  "))
	}
	q("columns", `SELECT format('%s %s %s %s', attname, format_type(atttypid, atttypmod), attnotnull,
	                     coalesce(pg_get_expr(d.adbin, d.adrelid), '-'))
	                FROM pg_attribute a LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
	               WHERE a.attrelid = 'public.agents'::regclass AND a.attnum > 0 AND NOT a.attisdropped`)
	q("column order", `SELECT string_agg(attname, ',' ORDER BY attnum)
	                     FROM pg_attribute WHERE attrelid = 'public.agents'::regclass AND attnum > 0 AND NOT attisdropped`)
	q("constraints", `SELECT conname || ' ' || pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid = 'public.agents'::regclass`)
	q("indexes", `SELECT pg_get_indexdef(indexrelid) FROM pg_index WHERE indrelid = 'public.agents'::regclass`)
	q("incoming fks", `SELECT conrelid::regclass::text || ' ' || conname || ' ' || pg_get_constraintdef(oid)
	                     FROM pg_constraint WHERE confrelid = 'public.agents'::regclass AND conrelid <> 'public.agents'::regclass`)
	q("table", `SELECT format('owner=%s acl=%s rls=%s/%s', pg_get_userbyid(relowner), coalesce(relacl::text, '-'),
	                          relrowsecurity, relforcerowsecurity)
	              FROM pg_class WHERE oid = 'public.agents'::regclass`)
	return strings.Join(parts, "\n")
}

func agentsData(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	var s sql.NullString
	if err := db.Get(&s, `SELECT json_agg(a ORDER BY id)::text FROM public.agents a`); err != nil {
		t.Fatal(err)
	}
	return s.String
}

// Running the full migration set again must not grow any table: every boot
// re-runs every file, so anything else burns attnums towards the 1600 limit.
func TestMigrations_RerunDoesNotGrowAnyTable(t *testing.T) {
	db := newMigrationDB(t)
	migrate(t, db)
	first := attnums(t, db)
	migrate(t, db)
	migrate(t, db)
	second := attnums(t, db)
	for tbl, a := range first {
		if second[tbl] != a {
			t.Errorf("%s: (max attnum, dropped) went %v -> %v after re-running migrations", tbl, a, second[tbl])
		}
	}
	if len(first) != len(second) {
		t.Errorf("table count changed on re-run: %d -> %d", len(first), len(second))
	}
}

// The staging state: agents at exactly 1600 attnums. Booting must succeed,
// give the attnums back and keep rows, constraints, indexes and the foreign
// keys of other tables pointing at agents.
func TestMigrations_AgentsAtColumnLimitIsRebuilt(t *testing.T) {
	ctx := context.Background()
	db := newMigrationDB(t)
	migrate(t, db)

	var projectID, agentID string
	if err := db.Get(&projectID, `INSERT INTO projects (name) VALUES ('spa8852') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&agentID, `
		INSERT INTO agents (project_id, name, handle, llm_provider, llm_model, llm_api_key_secret, system_prompt)
		VALUES ($1, 'Agent', 'agent', 'openai', 'gpt', 'secret-ref', 'be brief') RETURNING id`, projectID); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO agent_skills (agent_id, skill_name, skill_source) VALUES ($1, 's', 'inline')`, agentID)
	mustExec(t, db, `INSERT INTO agent_mcp_servers (agent_id, server_name, transport) VALUES ($1, 'm', 'http')`, agentID)

	// Burn attnums the way the boot loop did, up to the hard limit.
	mustExec(t, db, `DO $$ BEGIN
		WHILE (SELECT max(attnum) FROM pg_attribute WHERE attrelid = 'public.agents'::regclass) < 1600 LOOP
			ALTER TABLE agents ADD COLUMN spa8852_burn int;
			ALTER TABLE agents DROP COLUMN spa8852_burn;
		END LOOP; END $$`)
	if _, err := db.ExecContext(ctx, `ALTER TABLE agents ADD COLUMN one_more int`); !isSQLState(err, "54011") {
		t.Fatalf("setup: expected agents to be at the 1600-column limit (54011), got %v", err)
	}

	shapeBefore := agentsShape(t, db)
	dataBefore := agentsData(t, db)

	migrate(t, db) // the boot that crash-looped on staging

	if a := attnums(t, db)["agents"]; a[1] != 0 || a[0] > 40 {
		t.Fatalf("agents not rebuilt: max attnum %d, dropped %d", a[0], a[1])
	}
	if got := agentsShape(t, db); got != shapeBefore {
		t.Errorf("agents shape changed by the rebuild\nbefore:\n%s\nafter:\n%s", shapeBefore, got)
	}
	if got := agentsData(t, db); got != dataBefore {
		t.Errorf("agents rows changed by the rebuild\nbefore: %s\nafter:  %s", dataBefore, got)
	}
	var children int
	if err := db.Get(&children, `SELECT (SELECT count(*) FROM agent_skills) + (SELECT count(*) FROM agent_mcp_servers)`); err != nil {
		t.Fatal(err)
	}
	if children != 2 {
		t.Fatalf("child rows: got %d, want 2", children)
	}

	// The recreated foreign keys are live: orphans are refused, deletes cascade,
	// and the unique handle index still holds.
	if _, err := db.ExecContext(ctx, `INSERT INTO agent_skills (agent_id, skill_name, skill_source) VALUES (gen_random_uuid(), 'x', 'inline')`); !isSQLState(err, "23503") {
		t.Errorf("orphan agent_skills row: want foreign-key violation 23503, got %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO agents (project_id, name, handle, llm_provider, llm_model, llm_api_key_secret)
		VALUES ($1, 'Dup', 'agent', 'openai', 'gpt', 'x')`, projectID); !isSQLState(err, "23505") {
		t.Errorf("duplicate handle: want unique violation 23505, got %v", err)
	}
	mustExec(t, db, `DELETE FROM agents WHERE id = $1`, agentID)
	if err := db.Get(&children, `SELECT (SELECT count(*) FROM agent_skills) + (SELECT count(*) FROM agent_mcp_servers)`); err != nil {
		t.Fatal(err)
	}
	if children != 0 {
		t.Errorf("ON DELETE CASCADE lost: %d child rows left", children)
	}

	// And it is a no-op from now on.
	after := attnums(t, db)
	migrate(t, db)
	if again := attnums(t, db); again["agents"] != after["agents"] {
		t.Errorf("agents changed on the next boot: %v -> %v", after["agents"], again["agents"])
	}
}

// Below the threshold nothing happens: a rebuild takes an exclusive lock, not
// worth it for a few dropped columns.
func TestMigrations_AgentsBelowThresholdIsLeftAlone(t *testing.T) {
	db := newMigrationDB(t)
	migrate(t, db)
	before := agentsOID(t, db)
	burnAgentsAttnums(t, db, 50)
	migrate(t, db)
	if after := agentsOID(t, db); after != before {
		t.Errorf("agents was rebuilt with only ~50 dropped columns (oid %d -> %d)", before, after)
	}
}

func agentsOID(t *testing.T, db *sqlx.DB) int64 {
	t.Helper()
	var oid int64
	if err := db.Get(&oid, `SELECT 'public.agents'::regclass::oid::bigint`); err != nil {
		t.Fatal(err)
	}
	return oid
}

func burnAgentsAttnums(t *testing.T, db *sqlx.DB, n int) {
	t.Helper()
	mustExec(t, db, fmt.Sprintf(`DO $$ BEGIN FOR i IN 1..%d LOOP
		ALTER TABLE agents ADD COLUMN spa8852_burn int; ALTER TABLE agents DROP COLUMN spa8852_burn;
		END LOOP; END $$`, n))
}

// Something the rebuild cannot carry over (here a trigger) makes it skip with
// a NOTICE rather than lose it; the boot still succeeds.
func TestMigrations_AgentsRebuildSkipsUnknownDependents(t *testing.T) {
	db := newMigrationDB(t)
	migrate(t, db)
	mustExec(t, db, `CREATE FUNCTION spa8852_noop() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$`)
	mustExec(t, db, `CREATE TRIGGER spa8852_t BEFORE UPDATE ON agents FOR EACH ROW EXECUTE FUNCTION spa8852_noop()`)
	burnAgentsAttnums(t, db, 150)
	before := agentsOID(t, db)
	migrate(t, db)
	if after := agentsOID(t, db); after != before {
		t.Fatalf("agents rebuilt despite a trigger it cannot carry over (oid %d -> %d)", before, after)
	}
	var n int
	if err := db.Get(&n, `SELECT count(*) FROM pg_trigger WHERE tgname = 'spa8852_t'`); err != nil || n != 1 {
		t.Fatalf("trigger lost: count %d, err %v", n, err)
	}
}

func mustExec(t *testing.T, db *sqlx.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", strings.SplitN(strings.TrimSpace(query), "\n", 2)[0], err)
	}
}

func isSQLState(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// Two replicas booting at once: the second one runs 000033 while the first
// holds the lock and rebuilds agents. Once the first commits, the second must
// see the clean table and do nothing, not fail on the oid of the table that was
// just dropped (cross-check note on #63).
func TestMigrations_AgentsRebuildConcurrentBootIsNoOp(t *testing.T) {
	ctx := context.Background()
	db := newMigrationDB(t)
	migrate(t, db)
	burnAgentsAttnums(t, db, 150)

	body, err := fs.ReadFile(migrations.FS, "000033_rebuild_agents_reclaim_dropped_columns.sql")
	if err != nil {
		t.Fatal(err)
	}

	first, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback() }()
	if _, err := first.ExecContext(ctx, `LOCK TABLE public.agents IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}

	// The second boot reads the old oid, then waits on the lock.
	second := make(chan error, 1)
	go func() {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			second <- err
			return
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			second <- err
			return
		}
		second <- tx.Commit()
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := db.Get(&waiting, `SELECT count(*) FROM pg_locks WHERE relation = 'public.agents'::regclass AND NOT granted`); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second boot never queued on the agents lock")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := first.ExecContext(ctx, string(body)); err != nil {
		t.Fatalf("first boot rebuild: %v", err)
	}
	if err := first.Commit(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second boot failed after the first rebuilt agents: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("second boot did not finish")
	}
	if a := attnums(t, db)["agents"]; a[1] != 0 {
		t.Fatalf("agents still has %d dropped columns", a[1])
	}
}
