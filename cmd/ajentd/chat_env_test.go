package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadChatEnv(t *testing.T) {
	t.Setenv("ASK_MODEL", "existing-model")
	t.Setenv("ASK_DAILY_LIMIT", "9")
	t.Setenv("UNRELATED_TEST_SECRET", "existing")
	old, existed := os.LookupEnv("OPENROUTER_API_KEY")
	os.Unsetenv("OPENROUTER_API_KEY")
	defer func() {
		if existed {
			os.Setenv("OPENROUTER_API_KEY", old)
		} else {
			os.Unsetenv("OPENROUTER_API_KEY")
		}
	}()
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("# comment\nOPENROUTER_API_KEY='fake-key'\nASK_MODEL=other\nUNRELATED_TEST_SECRET=changed\n"), 0600)
	loadChatEnv(path)
	if os.Getenv("OPENROUTER_API_KEY") != "fake-key" || os.Getenv("ASK_MODEL") != "existing-model" || os.Getenv("UNRELATED_TEST_SECRET") != "existing" {
		t.Fatal("dotenv scope or precedence")
	}
}
