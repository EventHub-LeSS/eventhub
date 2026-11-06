package testdb

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

var migrationFile = regexp.MustCompile(`^(\d+)_(\w+)\.(up|down)\.sql$`)

// Branches that number their migrations independently must not reuse a version: golang-migrate
// refuses duplicate versions, and a database that already ran another migration with the same
// version would silently skip the new one.
func TestMigrationVersionsAreUniqueAndContiguous(t *testing.T) {
	if _, err := iofs.New(os.DirFS(migrationsDir()), "."); err != nil {
		t.Fatalf("migration source: %v", err)
	}
	entries, err := os.ReadDir(migrationsDir())
	if err != nil {
		t.Fatal(err)
	}
	names := map[int]string{}
	directions := map[int]map[string]bool{}
	for _, entry := range entries {
		match := migrationFile.FindStringSubmatch(entry.Name())
		if entry.IsDir() || match == nil {
			t.Errorf("unexpected file in migrations: %s", entry.Name())
			continue
		}
		version, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		if name, ok := names[version]; ok && name != match[2] {
			t.Errorf("version %d is used by %s and %s", version, name, match[2])
		}
		names[version] = match[2]
		if directions[version] == nil {
			directions[version] = map[string]bool{}
		}
		directions[version][match[3]] = true
	}
	for version := 1; version <= len(names); version++ {
		if _, ok := names[version]; !ok {
			t.Errorf("version %d is missing", version)
			continue
		}
		if !directions[version]["up"] || !directions[version]["down"] {
			t.Errorf("version %d needs an up and a down migration", version)
		}
	}
}
