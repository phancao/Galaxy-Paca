package migrations

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	createIndexRe = regexp.MustCompile(`(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:CONCURRENTLY\s+)?IF\s+NOT\s+EXISTS\s+("?[a-z0-9_]+"?)`)
	dropIndexRe   = regexp.MustCompile(`(?i)DROP\s+INDEX\s+(?:CONCURRENTLY\s+)?IF\s+EXISTS\s+("?[a-z0-9_]+"?)`)
	lineComment   = regexp.MustCompile(`--[^\n]*`)

	alterTableRe = regexp.MustCompile(`(?i)ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?("?[a-z0-9_.]+"?)`)
	addColumnRe  = regexp.MustCompile(`(?i)ADD\s+(?:COLUMN\s+)?IF\s+NOT\s+EXISTS\s+("?[a-z0-9_]+"?)`)
	dropColumnRe = regexp.MustCompile(`(?i)DROP\s+COLUMN\s+(?:IF\s+EXISTS\s+)?("?[a-z0-9_]+"?)`)
)

// TestMigrationsRerunSafe guards the "every file re-runs on every boot" model
// (RunMigrationsFS has no ledger). If a later file drops an index for good
// (drops it and never recreates it under the same name), no earlier file may
// still create that index: on the next boot the earlier file would rebuild the
// retired index — possibly with a stricter definition the live data no longer
// satisfies — and the API would fail to start. That is exactly how 000001
// rebuilt the full uni_users_username after 000016 had scoped it to active
// users (SPA-6550, SpaxeAI prod 07/11/2026).
func TestMigrationsRerunSafe(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	createdIn := map[string][]string{} // index -> files creating it
	retiredIn := map[string]string{}   // index -> file that drops it without recreating
	for _, n := range names {
		b, err := fs.ReadFile(FS, n)
		if err != nil {
			t.Fatal(err)
		}
		sql := lineComment.ReplaceAllString(string(b), "")
		created := map[string]bool{}
		for _, m := range createIndexRe.FindAllStringSubmatch(sql, -1) {
			idx := strings.ToLower(strings.Trim(m[1], `"`))
			created[idx] = true
			createdIn[idx] = append(createdIn[idx], n)
		}
		for _, m := range dropIndexRe.FindAllStringSubmatch(sql, -1) {
			idx := strings.ToLower(strings.Trim(m[1], `"`))
			if !created[idx] {
				retiredIn[idx] = n
			}
		}
	}

	for idx, dropFile := range retiredIn {
		for _, f := range createdIn[idx] {
			if f < dropFile {
				t.Errorf("%s retires index %q but earlier %s still creates it; on the next boot %s rebuilds it. Create the replacement index in %s instead.", dropFile, idx, f, f, f)
			}
		}
	}
}

// TestMigrationsDoNotReAddRetiredColumns is the column twin of the index guard
// above. If a later file drops a column for good, no earlier file may still
// `ADD COLUMN IF NOT EXISTS` it: every boot the earlier file re-adds the column
// and the later one drops it again, and Postgres never gives back the attnum of
// a dropped column (not even VACUUM FULL does). After ~1600 boots the table is
// at the 1600-column ceiling and every ADD COLUMN on it — including the earlier
// file's own — fails, so the API cannot start. That is how staging's
// galaxy-paca-api crash-looped on 000010 with 1582 dropped columns in agents
// (SPA-8755, 18/11/2026).
func TestMigrationsDoNotReAddRetiredColumns(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	addedIn := map[string][]string{} // table.column -> files adding it
	retiredIn := map[string]string{} // table.column -> file dropping it without re-adding
	for _, n := range names {
		b, err := fs.ReadFile(FS, n)
		if err != nil {
			t.Fatal(err)
		}
		added := map[string]bool{}
		var dropped []string
		for _, stmt := range strings.Split(lineComment.ReplaceAllString(string(b), ""), ";") {
			m := alterTableRe.FindStringSubmatch(stmt)
			if m == nil {
				continue
			}
			table := strings.TrimPrefix(strings.ToLower(strings.Trim(m[1], `"`)), "public.")
			for _, c := range addColumnRe.FindAllStringSubmatch(stmt, -1) {
				key := table + "." + strings.ToLower(strings.Trim(c[1], `"`))
				added[key] = true
				addedIn[key] = append(addedIn[key], n)
			}
			for _, c := range dropColumnRe.FindAllStringSubmatch(stmt, -1) {
				dropped = append(dropped, table+"."+strings.ToLower(strings.Trim(c[1], `"`)))
			}
		}
		for _, key := range dropped {
			if !added[key] {
				retiredIn[key] = n
			}
		}
	}

	for key, dropFile := range retiredIn {
		for _, f := range addedIn[key] {
			if f < dropFile {
				t.Errorf("%s drops column %q but earlier %s still adds it; every boot re-adds and re-drops it and burns attnums until the table hits the 1600-column limit. Remove the ADD COLUMN from %s.", dropFile, key, f, f)
			}
		}
	}
}
