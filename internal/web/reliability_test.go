package web

import (
	"os/exec"
	"testing"
)

func TestClientReliability(t *testing.T) {
	out, err := exec.Command("python3", "onboarding/reliability_test.py").CombinedOutput()
	if err != nil {
		t.Fatalf("client reliability: %v %s", err, out)
	}
}
