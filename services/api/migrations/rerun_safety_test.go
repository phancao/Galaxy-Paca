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
