package memory

import (
	"github.com/rs/zerolog"
)

// testLogger returns a no-op logger for tests.
func testLogger() zerolog.Logger {
	return zerolog.Nop()
}
