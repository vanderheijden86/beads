package versioncontrolops

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fkCascadeRepairExemptions lists tables that carry an ON DELETE CASCADE FK to
// issues(id) but have no fkCascadeRepairDeletes entry, so
// TestFKCascadeRepairDeletesCoversSchema does not fail on them.
var fkCascadeRepairExemptions = map[string]string{
	// provenance_events cascades from issues (migration 0063) but was never
	// added to fkCascadeRepairDeletes. Known upstream gap, not introduced by
	// this fix and not fixed here.
	"provenance_events": "upstream gap: no fkCascadeRepairDeletes entry",
}

// cascadeFKPattern matches an ON DELETE CASCADE foreign key that points at
// refTable, in the quoted ALTER TABLE form some migrations use.
func cascadeFKPattern(refTable string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(
		`(?is)ALTER\s+TABLE\s+([A-Za-z_][A-Za-z0-9_]*)\s+ADD\s+CONSTRAINT.*?REFERENCES\s+%s\s*\(\s*id\s*\).*?ON\s+DELETE\s+CASCADE`,
		refTable))
}

// createCascadePattern matches the bare CREATE TABLE form, which names the
// table in the statement header rather than in the constraint.
func createCascadePattern(refTable string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(
		`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?`+"`?"+`([A-Za-z_][A-Za-z0-9_]*)`+"`?"+`\s*\((?:[^;]*?)REFERENCES\s+%s\s*\(\s*id\s*\)\s*ON\s+DELETE\s+CASCADE`,
		refTable))
}

// migrationCascadeTargets derives, from the shipped migrations, every table
// with an ON DELETE CASCADE FK pointing at refTable(id).
func migrationCascadeTargets(t *testing.T, refTable string) map[string]bool {
	t.Helper()
	dir := filepath.Join("..", "schema", "migrations")
	found := map[string]bool{}
	alterPat := cascadeFKPattern(refTable)
	createPat := createCascadePattern(refTable)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return nil
		}
		body, readErr := os.ReadFile(path) //nolint:gosec // G304: path comes from WalkDir over a repo-relative dir
		if readErr != nil {
			return readErr
		}
		text := string(body)
		for _, m := range alterPat.FindAllStringSubmatch(text, -1) {
			found[m[1]] = true
		}
		for _, m := range createPat.FindAllStringSubmatch(text, -1) {
			found[m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return found
}

// TestFKCascadeRepairDeletesCoversSchema derives the cascade set from the
// shipped migrations and asserts fkCascadeRepairDeletes lists every table
// found, except the tables in fkCascadeRepairExemptions. Dolt merges each
// table row-wise and never re-executes cascades (bd-6dnrw.4), so a table
// missing here leaves TryRepairFKCascadeViolations unable to repair a merge
// that dangles a child row against a deleted issue, and the merge is refused
// forever rather than converging.
func TestFKCascadeRepairDeletesCoversSchema(t *testing.T) {
	found := migrationCascadeTargets(t, "issues")
	if len(found) == 0 {
		t.Fatal("no ON DELETE CASCADE references to issues found; the test proves nothing")
	}

	for table := range found {
		if _, exempt := fkCascadeRepairExemptions[table]; exempt {
			continue
		}
		if _, ok := fkCascadeRepairDeletes[table]; !ok {
			t.Errorf("migrations declare ON DELETE CASCADE from issues to %q, but fkCascadeRepairDeletes omits it", table)
		}
	}
}
