//go:build provider_metadata

package main

// Retain the original pinned provider package for dependency metadata and
// module maintenance without compiling its generated SDK in production or CI.
import _ "github.com/pulumi/pulumi-cloudflare/sdk/v6/go/cloudflare"
