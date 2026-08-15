package testutil

import (
	"fmt"
	"os"
	"testing"
)

// RunTests is the TestMain body every package that touches infrastructure
// should use:
//
//	func TestMain(m *testing.M) { testutil.RunTests(m) }
//
// It exists because of a specific trap: SetupTestDB and RequireTestRedis skip
// when TEST_DATABASE_URL / TEST_REDIS_URL are unset, and nearly every test in
// this repo needs one or both. A bare `go test ./...` therefore prints a
// confident PASS while running almost nothing — the failure mode is a green
// build that proves nothing. This prints a banner loud enough to notice.
func RunTests(m *testing.M) {
	if missing := missingInfra(); len(missing) > 0 {
		banner(missing)
	}
	os.Exit(m.Run())
}

// missingInfra returns the names of the unset infrastructure variables.
func missingInfra() []string {
	var missing []string
	if os.Getenv("TEST_DATABASE_URL") == "" {
		missing = append(missing, "TEST_DATABASE_URL")
	}
	if os.Getenv("TEST_REDIS_URL") == "" {
		missing = append(missing, "TEST_REDIS_URL")
	}
	return missing
}

func banner(missing []string) {
	const line = "════════════════════════════════════════════════════════════════════"
	fmt.Fprintf(os.Stderr, "\n%s\n", line)
	fmt.Fprintf(os.Stderr, "  WARNING: infrastructure-backed tests will be SKIPPED\n")
	for _, name := range missing {
		fmt.Fprintf(os.Stderr, "    unset: %s\n", name)
	}
	fmt.Fprintf(os.Stderr, "\n  A PASS from this run does not mean the suite passed.\n")
	fmt.Fprintf(os.Stderr, "  Start the services and re-run, for example:\n\n")
	fmt.Fprintf(os.Stderr, "    TEST_DATABASE_URL=\"host=localhost port=5432 user=whatomate \\\n")
	fmt.Fprintf(os.Stderr, "      password=whatomate dbname=whatomate_test sslmode=disable\" \\\n")
	fmt.Fprintf(os.Stderr, "    TEST_REDIS_URL=\"redis://localhost:6379/1\" \\\n")
	fmt.Fprintf(os.Stderr, "    go test -race -p 1 ./...\n")
	fmt.Fprintf(os.Stderr, "%s\n\n", line)
}
