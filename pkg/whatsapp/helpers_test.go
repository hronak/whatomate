package whatsapp_test

import (
	"github.com/zerodha/logf"
)

// nopLogger returns a logger that stays quiet during tests.
//
// Defined here rather than borrowed from test/testutil: this package is meant
// to be usable outside the module, and its tests should not depend on the
// module's internal test helpers to prove it.
func nopLogger() logf.Logger {
	return logf.New(logf.Opts{
		Level:        logf.ErrorLevel,
		EnableCaller: false,
		EnableColor:  false,
	})
}
