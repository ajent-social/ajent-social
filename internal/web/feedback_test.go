package web

import (
	"os/exec"
	"testing"
)

func TestClientFeedback(t *testing.T) {
	out, err := exec.Command("python3", "onboarding/feedback_test.py").CombinedOutput()
	if err != nil {
		t.Fatalf("client feedback suite: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}
