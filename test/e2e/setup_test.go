package e2e

import (
	"net/http"
	"os"
	"time"
)

// TestEnv holds the test environment configuration.
type TestEnv struct {
	BaseURL string
	Client  *http.Client
}

var testEnv *TestEnv

// GetTestEnv returns the current test environment.
func GetTestEnv() *TestEnv {
	if testEnv == nil {
		baseURL := os.Getenv("MOTE_TEST_URL")
		if baseURL == "" {
			baseURL = "http://localhost:18788"
		}

		testEnv = &TestEnv{
			BaseURL: baseURL,
			Client: &http.Client{
				Timeout: 10 * time.Second,
			},
		}
	}
	return testEnv
}

// SetTestEnv sets the test environment for testing.
func SetTestEnv(env *TestEnv) {
	testEnv = env
}

// ResetTestEnv resets the test environment.
func ResetTestEnv() {
	testEnv = nil
}
