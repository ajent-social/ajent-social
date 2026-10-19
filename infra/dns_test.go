package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type dnsContractMocks struct{ registration *pulumi.MockResourceArgs }

func (m *dnsContractMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

func (m *dnsContractMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken == "cloudflare:index/dnsRecord:DnsRecord" {
		m.registration = &args
	}
	return args.Name + "-id", args.Inputs, nil
}

func TestDNSRegistrationPreservesProviderContract(t *testing.T) {
	for _, tc := range []struct{ kind, content string }{{"A", "192.0.2.1"}, {"CNAME", "target.example.test"}} {
		t.Run(tc.kind, func(t *testing.T) {
			m := &dnsContractMocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				return registerProductionDNS(ctx, "zone-test", "service.example.test", tc.kind, pulumi.String(tc.content))
			}, pulumi.WithMocks("project", "stack", m))
			if err != nil {
				t.Fatal(err)
			}
			a := m.registration
			if a == nil || a.RegisterRPC == nil {
				t.Fatal("missing DNS registration")
			}
			if a.Name != "production-dns" || !a.Custom {
				t.Fatal("resource identity changed")
			}
			if a.RegisterRPC.GetVersion() != "6.20.0" {
				t.Fatal("provider version changed")
			}
			want := map[string]any{"zoneId": "zone-test", "name": "service.example.test", "type": tc.kind,
				"content": tc.content, "ttl": float64(300), "proxied": false, "comment": "Ajent service; DNS-only"}
			if !reflect.DeepEqual(a.Inputs.Mappable(), want) {
				t.Fatal("DNS inputs changed")
			}
			alias, typeAlias := false, false
			for _, v := range a.RegisterRPC.GetAliases() {
				if v.GetSpec().GetType() == "cloudflare:index/record:Record" || strings.Contains(v.GetUrn(), "::cloudflare:index/record:Record::") {
					typeAlias = true
				}
				if v.GetSpec().GetName() == "dogfood-dns" || strings.HasSuffix(v.GetUrn(), "::dogfood-dns") {
					alias = true
				}
			}
			for _, v := range a.RegisterRPC.GetAliasURNs() {
				if strings.Contains(v, "::cloudflare:index/record:Record::") {
					typeAlias = true
				}
				if strings.HasSuffix(v, "::dogfood-dns") {
					alias = true
				}
			}
			if !typeAlias {
				t.Fatal("historical resource type alias lost")
			}
			if !alias {
				t.Fatal("historical resource alias lost")
			}
		})
	}
}
