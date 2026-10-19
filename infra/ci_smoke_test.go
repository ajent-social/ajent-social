//go:build infra_ci

package main

import "testing"

func TestCIBuildExcludesProviderSDKs(t *testing.T) {
	// Compiling this package under -tags infra_ci must not pull generated
	// pulumi-aws SDKs onto the shared runner. DNS uses the same lightweight
	// registration and pinned real provider contract in both build modes.
	if err := validateImprovementConfig("ajent-social/ajent-social", ""); err != nil {
		t.Fatal(err)
	}
}
