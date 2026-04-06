package scoring

import (
	"cullsnap/internal/logger"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	logger.Init("/dev/null") //nolint:errcheck // test init
	os.Exit(m.Run())
}
