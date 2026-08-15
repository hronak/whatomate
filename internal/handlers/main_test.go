package handlers_test

import (
	"testing"

	"github.com/shridarpatil/whatomate/test/testutil"
)

// TestMain warns loudly when the infrastructure these tests need is absent,
// so a skipped run is not mistaken for a passing one.
func TestMain(m *testing.M) {
	testutil.RunTests(m)
}
