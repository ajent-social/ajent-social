//go:build !infra_ci

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type capturedResource struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Inputs map[string]any `json:"inputs"`
}
type infraMocks struct {
	mu                  sync.Mutex
	resources           []capturedResource
	protected           map[string]bool
	secretInputs        map[string]bool
	dnsAlias            bool
	ignoredCapacity     bool
	rollingDependencies int
	rejectParameter     bool
	parameterCalls      int
}

func (m *infraMocks) Call(a pulumi.MockCallArgs) (resource.PropertyMap, error) {
	switch {
	case strings.Contains(a.Token, "getAvailabilityZones"):
		return resource.NewPropertyMapFromMap(map[string]any{"names": []string{"us-west-2a", "us-west-2b"}}), nil
	case strings.Contains(a.Token, "getParameter"):
		m.mu.Lock()
		defer m.mu.Unlock()
		m.parameterCalls++
		if m.rejectParameter {
			return nil, fmt.Errorf("unexpected SSM lookup")
		}
		return resource.NewPropertyMapFromMap(map[string]any{"value": "ami-test"}), nil
	default:
		return a.Args, nil
	}
}

// Preserve secret flags separately; normalize their envelopes for cross-SDK comparison.
func unwrapSecret(v resource.PropertyValue) (any, bool) {
	if v.IsSecret() {
		return v.SecretValue().Element.MapRepl(nil, unwrapSecret), true
	}
	return nil, false
}

func (m *infraMocks) NewResource(a pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, capturedResource{a.TypeToken, a.Name, a.Inputs.MapRepl(nil, unwrapSecret)})
	for _, key := range []resource.PropertyKey{"password", "secretString"} {
		if value, ok := a.Inputs[key]; ok {
			m.secretInputs[a.TypeToken+"/"+a.Name+"/"+string(key)] = value.IsSecret()
		}
	}
	if a.RegisterRPC != nil {
		m.protected[a.TypeToken+"/"+a.Name] = a.RegisterRPC.GetProtect()
		if a.TypeToken == "cloudflare:index/dnsRecord:DnsRecord" {
			for _, alias := range a.RegisterRPC.GetAliases() {
				if alias.GetSpec().GetName() == "dogfood-dns" || strings.HasSuffix(alias.GetUrn(), "::dogfood-dns") {
					m.dnsAlias = true
				}
			}
			for _, urn := range a.RegisterRPC.GetAliasURNs() {
				if strings.HasSuffix(urn, "::dogfood-dns") {
					m.dnsAlias = true
				}
			}
		}
		if a.TypeToken == "aws:autoscaling/group:Group" {
			m.ignoredCapacity = reflect.DeepEqual(a.RegisterRPC.GetIgnoreChanges(), []string{"desiredCapacity"})
			m.rollingDependencies = len(a.RegisterRPC.GetDependencies())
		}
	}
	state := a.Inputs.Copy()
	for key, value := range map[string]any{"arn": "arn:aws:test::123:" + a.Name, "address": "database.test", "repositoryUrl": "123.dkr.ecr.us-west-2.amazonaws.com/ajent", "publicIp": "192.0.2.1", "result": "fake-secret-for-test", "name": a.Name, "latestVersion": 1, "arnSuffix": a.Name, "identifier": a.Name, "dnsName": "edge.example", "domainValidationOptions": []any{}} {
		state[resource.PropertyKey(key)] = resource.NewPropertyValue(value)
	}
	// Model the engine's handling of SDK-requested secret outputs. The Go mock
	// monitor does not apply AdditionalSecretOutputs itself.
	if a.RegisterRPC != nil {
		for _, name := range a.RegisterRPC.GetAdditionalSecretOutputs() {
			key := resource.PropertyKey(name)
			if value, ok := state[key]; ok && !value.IsSecret() {
				state[key] = resource.MakeSecret(value)
			}
		}
	}
	return a.Name + "_id", state, nil
}

// Policy object ordering is insignificant. Names, arrays and bootstrap bytes are exact.
func normalized(v any) any {
	switch x := v.(type) {
	case map[string]any:
		if x["4dabf18193072939515e22adb298388d"] == "1b47061264138c4ac30d75fd1eb44270" {
			return normalized(x["value"])
		}
		out := map[string]any{}
		for k, v := range x {
			out[k] = normalized(v)
		}
		return out
	case []any:
		for i := range x {
			x[i] = normalized(x[i])
		}
		return x
	case string:
		var obj any
		if strings.HasPrefix(x, "{") && json.Unmarshal([]byte(x), &obj) == nil {
			return normalized(obj)
		}
	}
	return v
}
func jsonValue(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var obj any
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatal(err)
	}
	return normalized(obj)
}
func TestTypeScriptResourceParity(t *testing.T) {
	for _, mode := range []string{"foundation", "legacy", "transition", "rolling", "cutover", "apex"} {
		t.Run(mode, func(t *testing.T) {
			s := settings{Hostname: "dogfood.ajent.social", Region: "us-west-2", Stack: "test"}
			if mode != "foundation" {
				s.ImageDigest = "sha256:" + strings.Repeat("a", 64)
				s.CaddyImage = "caddy@sha256:" + strings.Repeat("b", 64)
				s.CloudflareZoneID = "test-zone"
				s.OpenrouterSecretARN = "arn:chat-test"
				s.AlertEmail = "operator@example.test"
			}
			s.RollingService = mode == "rolling" || mode == "transition" || mode == "cutover" || mode == "apex"
			s.RetireLegacyHost = s.RollingService && mode != "transition"
			if mode == "cutover" || mode == "apex" {
				s.CanonicalHostname = "ajent.social"
			}
			if mode == "cutover" {
				s.Hostname = "ajent.social"
				s.CertificateARN = "arn:apex-test"
			}
			mocks := &infraMocks{protected: map[string]bool{}, secretInputs: map[string]bool{}}
			if err := pulumi.RunErr(func(ctx *pulumi.Context) error { return deploy(ctx, s) }, pulumi.WithMocks("ajent-infra", "test", mocks)); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile("testdata/" + mode + "-resources.json")
			if err != nil {
				t.Fatal(err)
			}
			var want []capturedResource
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			var got []capturedResource
			for _, r := range mocks.resources {
				if r.Type != "pulumi:pulumi:Stack" {
					got = append(got, r)
				}
			}
			less := func(a, b capturedResource) bool { return a.Type+a.Name < b.Type+b.Name }
			sort.Slice(got, func(i, j int) bool { return less(got[i], got[j]) })
			sort.Slice(want, func(i, j int) bool { return less(want[i], want[j]) })
			if len(got) != len(want) {
				t.Fatalf("resource count: Go %d, TypeScript %d", len(got), len(want))
			}
			for i := range want {
				if got[i].Type != want[i].Type || got[i].Name != want[i].Name {
					t.Errorf("identity: got %s/%s want %s/%s", got[i].Type, got[i].Name, want[i].Type, want[i].Name)
					continue
				}
				if !reflect.DeepEqual(jsonValue(t, got[i].Inputs), jsonValue(t, want[i].Inputs)) {
					t.Errorf("inputs differ for %s/%s\nGo: %#v\nTypeScript: %#v", got[i].Type, got[i].Name, jsonValue(t, got[i].Inputs), jsonValue(t, want[i].Inputs))
				}
			}
			for _, name := range []string{"aws:rds/instance:Instance/database", "aws:secretsmanager/secret:Secret/database-secret", "aws:secretsmanager/secret:Secret/owner-secret"} {
				if !mocks.protected[name] {
					t.Errorf("lost protection: %s", name)
				}
			}
			for _, name := range []string{"aws:rds/instance:Instance/database/password", "aws:secretsmanager/secretVersion:SecretVersion/database-secret-value/secretString", "aws:secretsmanager/secretVersion:SecretVersion/owner-secret-value/secretString"} {
				if !mocks.secretInputs[name] {
					t.Errorf("lost secret propagation: %s", name)
				}
			}

			if s.CloudflareZoneID != "" && !mocks.dnsAlias {
				t.Error("production DNS lost its historical resource alias")
			}
			if s.RollingService {
				if !mocks.protected["aws:acm/certificate:Certificate/public-tls"] {
					t.Error("certificate lost protection")
				}
				if !mocks.ignoredCapacity {
					t.Error("autoscaler desired capacity must remain ignored")
				}
				if mocks.rollingDependencies < 8 {
					t.Errorf("fleet lost dependencies: %d", mocks.rollingDependencies)
				}
			}
		})
	}
}
func TestConfigurationValidation(t *testing.T) {
	valid := settings{Hostname: "dogfood.ajent.social", Region: "us-west-2", ImageDigest: "sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64), RollingService: true, RetireLegacyHost: true}
	for _, tc := range []struct {
		name    string
		mutate  func(*settings)
		wantErr bool
	}{
		{"valid", func(*settings) {}, false},
		{"invalid canonical hostname", func(s *settings) { s.CanonicalHostname = "other.example" }, true}, {"foundation", func(s *settings) { s.ImageDigest = ""; s.CaddyImage = "" }, false},
		{"region required", func(s *settings) { s.Region = "" }, true}, {"lowercase hostname", func(s *settings) { s.Hostname = "Dogfood.ajent.social" }, true}, {"shell injection", func(s *settings) { s.Hostname = "$(id).example.com" }, true}, {"digest pinned", func(s *settings) { s.ImageDigest = "latest" }, true}, {"caddy pinned", func(s *settings) { s.CaddyImage = "caddy:latest" }, true}, {"retirement guard", func(s *settings) { s.RollingService = false }, true},
		{"improvement repository", func(s *settings) { s.ImprovementRepository = "ajent-social/ajent-social" }, false},
		{"invalid improvement repository", func(s *settings) { s.ImprovementRepository = "Owner/repo" }, true},
		{"secret requires repository", func(s *settings) { s.ImprovementGitHubSecretARN = "arn:aws:secretsmanager:us-west-2:123456789012:secret:token" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.mutate(&s)
			err := s.validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validation error=%v; want error=%v", err, tc.wantErr)
			}
		})
	}
}

func TestImprovementWorkerDisabledHasNoAlarmsOrSecretAccess(t *testing.T) {
	s := settings{Hostname: "dogfood.ajent.social", Region: "us-west-2", Stack: "test", ImageDigest: "sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64)}
	mocks := &infraMocks{protected: map[string]bool{}, secretInputs: map[string]bool{}}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { return deploy(ctx, s) }, pulumi.WithMocks("ajent-infra", "disabled-test", mocks)); err != nil {
		t.Fatal(err)
	}
	for _, r := range mocks.resources {
		if strings.HasPrefix(r.Name, "improvement-worker-") {
			t.Fatalf("unexpected improvement resource %s/%s", r.Type, r.Name)
		}
	}
	var runtime *capturedResource
	for i := range mocks.resources {
		if mocks.resources[i].Name == "runtime" && mocks.resources[i].Type == "aws:iam/rolePolicy:RolePolicy" {
			runtime = &mocks.resources[i]
			break
		}
	}
	if runtime == nil {
		t.Fatal("runtime policy missing")
	}
	policy := jsonValue(t, runtime.Inputs["policy"]).(map[string]any)
	statements := policy["Statement"].([]any)
	var secretAccess map[string]any
	for _, raw := range statements {
		stmt := raw.(map[string]any)
		actions := stmt["Action"].([]any)
		for _, action := range actions {
			if action == "secretsmanager:GetSecretValue" {
				secretAccess = stmt
			}
		}
	}
	if secretAccess == nil {
		t.Fatal("secret access statement missing")
	}
	got := jsonValue(t, secretAccess["Resource"])
	want := jsonValue(t, []string{"arn:aws:test::123:database-secret", "arn:aws:test::123:owner-secret"})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("secret resources=%#v want %#v", got, want)
	}
}

func TestImprovementWorkerAlarmsAndBootstrap(t *testing.T) {
	s := settings{
		Hostname: "dogfood.ajent.social", Region: "us-west-2", Stack: "test",
		ImageDigest: "sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64),
		OpenrouterSecretARN: "arn:chat-test", ImprovementRepository: "ajent-social/ajent-social",
		ImprovementGitHubSecretARN: "arn:aws:secretsmanager:us-west-2:123456789012:secret:improvement-test",
		RollingService: true, RetireLegacyHost: true,
	}
	mocks := &infraMocks{protected: map[string]bool{}, secretInputs: map[string]bool{}}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { return deploy(ctx, s) }, pulumi.WithMocks("ajent-infra", "improvement-test", mocks)); err != nil {
		t.Fatal(err)
	}
	var heartbeat, stalled, overdue *capturedResource
	for i := range mocks.resources {
		r := &mocks.resources[i]
		switch r.Name {
		case "improvement-worker-health":
			heartbeat = r
		case "improvement-worker-stalled":
			stalled = r
		case "improvement-worker-overdue":
			overdue = r
		}
	}
	if heartbeat == nil || stalled == nil || overdue == nil {
		t.Fatal("improvement worker filters/alarms missing")
	}
	if !strings.Contains(fmt.Sprint(heartbeat.Inputs["pattern"]), "improvement_worker") {
		t.Fatalf("unexpected heartbeat pattern: %#v", heartbeat.Inputs["pattern"])
	}
	if stalled.Inputs["period"] != 60.0 && stalled.Inputs["period"] != float64(60) && stalled.Inputs["period"] != 60 {
		// pulumi mocks may deliver ints or float64
		if fmt.Sprint(stalled.Inputs["period"]) != "60" {
			t.Fatalf("stalled period=%#v", stalled.Inputs["period"])
		}
	}
	if fmt.Sprint(stalled.Inputs["evaluationPeriods"]) != "5" || fmt.Sprint(stalled.Inputs["datapointsToAlarm"]) != "5" {
		t.Fatalf("stalled windows=%#v %#v", stalled.Inputs["evaluationPeriods"], stalled.Inputs["datapointsToAlarm"])
	}
	if stalled.Inputs["statistic"] != "Minimum" || stalled.Inputs["treatMissingData"] != "breaching" {
		t.Fatalf("stalled statistic/missing=%#v %#v", stalled.Inputs["statistic"], stalled.Inputs["treatMissingData"])
	}
	if fmt.Sprint(overdue.Inputs["threshold"]) != "900" || overdue.Inputs["statistic"] != "Maximum" || overdue.Inputs["treatMissingData"] != "notBreaching" {
		t.Fatalf("overdue inputs=%#v", overdue.Inputs)
	}
	var runtime *capturedResource
	var template *capturedResource
	for i := range mocks.resources {
		r := &mocks.resources[i]
		if r.Name == "runtime" && r.Type == "aws:iam/rolePolicy:RolePolicy" {
			runtime = r
		}
		if r.Name == "application-fleet" && r.Type == "aws:ec2/launchTemplate:LaunchTemplate" {
			template = r
		}
	}
	if runtime == nil || template == nil {
		t.Fatal("runtime policy or launch template missing")
	}
	policy := jsonValue(t, runtime.Inputs["policy"]).(map[string]any)
	var secretAccess map[string]any
	for _, raw := range policy["Statement"].([]any) {
		stmt := raw.(map[string]any)
		for _, action := range stmt["Action"].([]any) {
			if action == "secretsmanager:GetSecretValue" {
				secretAccess = stmt
			}
		}
	}
	resources := jsonValue(t, secretAccess["Resource"]).([]any)
	if resources[len(resources)-1] != s.ImprovementGitHubSecretARN {
		t.Fatalf("improvement secret not last resource: %#v", resources)
	}
	encoded := template.Inputs["userData"].(string)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	launch := string(raw)
	files := regexp.MustCompile(`printf '%s' '([^']+)' \| base64 -d > (\S+)`).FindAllStringSubmatch(launch, -1)
	var configJSON []byte
	for _, file := range files {
		if strings.HasSuffix(file[2], "config.json") {
			configJSON, err = base64.StdEncoding.DecodeString(file[1])
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if configJSON == nil {
		t.Fatal("config.json missing from bootstrap")
	}
	var config map[string]any
	if err := json.Unmarshal(configJSON, &config); err != nil {
		t.Fatal(err)
	}
	if config["improvementRepository"] != "ajent-social/ajent-social" || config["improvementGitHubSecretArn"] != s.ImprovementGitHubSecretARN {
		t.Fatalf("bootstrap config=%#v", config)
	}
}

func TestLoadSettings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		extra   map[string]string
		wantErr bool
	}{
		{"defaults", nil, false},
		{"pinned host AMI", map[string]string{"hostAmi": "ami-0123456789abcdef0"}, false},
		{"invalid host AMI", map[string]string{"hostAmi": "ami-latest"}, true},
		{"rolling flags", map[string]string{"rollingService": "true", "retireLegacyHost": "true"}, false},
		{"empty hostname", map[string]string{"hostname": ""}, true},
		{"invalid boolean", map[string]string{"rollingService": "1"}, true},
		{"empty boolean", map[string]string{"rollingService": ""}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"aws:region": "us-west-2"}
			for k, v := range tc.extra {
				values["ajent-infra:"+k] = v
			}
			mocks := &infraMocks{protected: map[string]bool{}, secretInputs: map[string]bool{}}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				s, err := loadSettings(ctx)
				if tc.wantErr {
					if err == nil {
						t.Error("invalid config accepted")
					}
					return nil
				}
				if err != nil {
					return err
				}
				if s.HostAMI != tc.extra["hostAmi"] {
					t.Errorf("hostAmi not loaded: %q", s.HostAMI)
				}
				if s.Hostname != "ajent.social" || s.Region != "us-west-2" || s.Stack != "test" {
					t.Errorf("unexpected defaults: %+v", s)
				}
				return nil
			}, pulumi.WithMocks("ajent-infra", "test", mocks), func(info *pulumi.RunInfo) { info.Config = values })
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Exercise the real production deploy program, not the infra_ci substitute.
func TestPinnedHostAMIPreservesLaunchTemplateWithoutSSM(t *testing.T) {
	s := settings{Hostname: "ajent.social", Region: "us-west-2", Stack: "test", HostAMI: "ami-0123456789abcdef0", ImageDigest: "sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64), RollingService: true, RetireLegacyHost: true}
	mocks := &infraMocks{protected: map[string]bool{}, secretInputs: map[string]bool{}, rejectParameter: true}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { return deploy(ctx, s) }, pulumi.WithMocks("ajent-infra", "test", mocks)); err != nil {
		t.Fatal(err)
	}
	if mocks.parameterCalls != 0 {
		t.Fatalf("SSM calls = %d", mocks.parameterCalls)
	}
	count := 0
	for _, r := range mocks.resources {
		if r.Type == "aws:ec2/launchTemplate:LaunchTemplate" && r.Name == "application-fleet" {
			count++
			if r.Inputs["imageId"] != s.HostAMI {
				t.Fatalf("imageId = %v, want %s", r.Inputs["imageId"], s.HostAMI)
			}
		}
	}
	if count != 1 {
		t.Fatalf("launch templates = %d, want 1", count)
	}
}
