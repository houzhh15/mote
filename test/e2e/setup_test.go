package e2e
package e2e

import (
	"net/http"
	"os"
	"time"
)




































}	testEnv = nilfunc ResetTestEnv() {// ResetTestEnv resets the test environment.}	testEnv = envfunc SetTestEnv(env *TestEnv) {// SetTestEnv sets the test environment for testing.}	return testEnv	}		}			},				Timeout: 10 * time.Second,			Client: &http.Client{			BaseURL: baseURL,		testEnv = &TestEnv{		}			baseURL = "http://localhost:18788"		if baseURL == "" {		baseURL := os.Getenv("MOTE_TEST_URL")	if testEnv == nil {func GetTestEnv() *TestEnv {// GetTestEnv returns the current test environment.var testEnv *TestEnv}	Client  *http.Client	BaseURL stringtype TestEnv struct {// TestEnv holds the test environment configuration.