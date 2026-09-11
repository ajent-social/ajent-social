//go:build infra_ci

package main

import "testing"

func TestCIBuildExcludesProviderSDKs(t *testing.T) {
	// Compiling this package under -tags infra_ci must not pull generated
	// pulumi-aws / pulumi-cloudflare SDKs onto the shared runner.
	if err := validateImprovementConfig("ajent-social/ajent-social", ""); err != nil {
		t.Fatal(err)
	}
}
