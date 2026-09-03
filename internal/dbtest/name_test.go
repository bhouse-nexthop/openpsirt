package dbtest

import (
	"strings"
	"testing"
)

func TestTwoCheckoutsOfOnePackageGetDifferentDatabases(t *testing.T) {
	// The import path is the same in every checkout of this repository, so a
	// name made from it alone was the same too — and a second worktree run
	// against the same servers dropped the first one's database mid-run.
	// The directory a package is tested from is what tells checkouts apart.
	path := "github.com/bhouse-nexthop/openpsirt/internal/httpapi.test"
	one := databaseName(path, "/home/somebody/git/openpsirt/internal/httpapi")
	two := databaseName(path, "/home/somebody/git/openpsirt-2/internal/httpapi")
	if one == two {
		t.Fatalf("two checkouts of one package share the database %q", one)
	}
	for _, name := range []string{one, two} {
		// The readable part stays: somebody listing the server's databases
		// should see which package each belongs to.
		if !strings.HasPrefix(name, "openpsirt_t_httpapi_") {
			t.Errorf("%q does not name its package", name)
		}
		// Every engine's identifier limit is at least 63 characters.
		if len(name) > 63 {
			t.Errorf("%q is too long for an identifier", name)
		}
	}
	if again := databaseName(path, "/home/somebody/git/openpsirt/internal/httpapi"); again != one {
		t.Errorf("the name is not stable between runs: %q then %q", one, again)
	}
}
