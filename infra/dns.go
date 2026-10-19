package main

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

const cloudflareProviderVersion = "6.20.0"

// DNS needs one resource registration, not the generated SDK for every
// Cloudflare product. The real pinned provider still handles all effects.
type productionDNSRecord struct {
	pulumi.CustomResourceState
}

func registerProductionDNS(ctx *pulumi.Context, zoneID, hostname, kind string, content pulumi.StringInput) error {
	var record productionDNSRecord
	return ctx.RegisterResource("cloudflare:index/dnsRecord:DnsRecord", "production-dns", pulumi.Map{
		"zoneId":  pulumi.String(zoneID),
		"name":    pulumi.String(hostname),
		"type":    pulumi.String(kind),
		"content": content,
		"ttl":     pulumi.Float64(300),
		"proxied": pulumi.Bool(false),
		"comment": pulumi.String("Ajent service; DNS-only"),
	}, &record, pulumi.Version(cloudflareProviderVersion),
		pulumi.Aliases([]pulumi.Alias{
			{Name: pulumi.String("dogfood-dns")},
			{Type: pulumi.String("cloudflare:index/record:Record")},
		}))
}
