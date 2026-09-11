//go:build infra_ci

package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CI builds omit the generated pulumi-cloudflare SDK (it OOM-kills the shared
// runner). Register the same type token and inputs so mocks and parity tests
// still exercise production DNS identity, including the historical alias.
type ciDNSRecord struct {
	pulumi.CustomResourceState
}

func registerProductionDNS(ctx *pulumi.Context, zoneID, hostname, kind string, content pulumi.StringInput) error {
	var resource ciDNSRecord
	return ctx.RegisterResource("cloudflare:index/dnsRecord:DnsRecord", "production-dns", pulumi.Map{
		"zoneId":  pulumi.String(zoneID),
		"name":    pulumi.String(hostname),
		"type":    pulumi.String(kind),
		"content": content,
		"ttl":     pulumi.Float64(300),
		"proxied": pulumi.Bool(false),
		"comment": pulumi.String("Ajent service; DNS-only"),
	}, &resource, pulumi.Aliases([]pulumi.Alias{{Name: pulumi.String("dogfood-dns")}}))
}
