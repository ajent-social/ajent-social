//go:build !infra_ci

package main

import (
	"github.com/pulumi/pulumi-cloudflare/sdk/v6/go/cloudflare"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func registerProductionDNS(ctx *pulumi.Context, zoneID, hostname, kind string, content pulumi.StringInput) error {
	_, err := cloudflare.NewDnsRecord(ctx, "production-dns", &cloudflare.DnsRecordArgs{
		ZoneId:  pulumi.String(zoneID),
		Name:    pulumi.String(hostname),
		Type:    pulumi.String(kind),
		Content: content,
		Ttl:     pulumi.Float64(300),
		Proxied: pulumi.Bool(false),
		Comment: pulumi.String("Ajent service; DNS-only"),
	}, pulumi.Aliases([]pulumi.Alias{{Name: pulumi.String("dogfood-dns")}}))
	return err
}
