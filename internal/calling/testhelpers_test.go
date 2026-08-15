package calling

import "github.com/zerodha/logf"

// nopLogger returns a logger that stays quiet during tests.
func nopLogger() logf.Logger {
	return logf.New(logf.Opts{Level: logf.ErrorLevel, EnableCaller: false, EnableColor: false})
}
