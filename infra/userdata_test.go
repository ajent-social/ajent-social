package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestBootstrapParityAndSyntax(t *testing.T) {
	for _, mode := range []string{"legacy", "rolling", "cutover"} {
		t.Run(mode, func(t *testing.T) {
			c := hostConfig{Hostname: "dogfood.ajent.social", Region: "us-west-2", DBHost: "db.example", DBARN: "arn:db", OwnerARN: "arn:owner", Image: "registry/app@sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64), LogGroup: "logs", Stack: "dogfood"}
			if mode != "legacy" {
				c.BehindLoadBalancer = true
				c.OpenrouterSecretARN = "arn:chat-test"
			}
			if mode == "cutover" {
				c.CanonicalHostname = "ajent.social"
				c.LegacyOrigin = "https://dogfood.ajent.social"
			}
			script, err := bootstrap(c)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile("testdata/" + mode + ".sh")
			if err != nil {
				t.Fatal(err)
			}
			if script != string(want) {
				t.Fatal("bootstrap differs from TypeScript bytes; migration would roll hosts")
			}
			if len(script) >= 16384 {
				t.Fatalf("EC2 user data too large: %d", len(script))
			}
			cmd := exec.Command("bash", "-n")
			cmd.Stdin = strings.NewReader(script)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("shell syntax: %v %s", err, out)
			}
			files := regexp.MustCompile(`printf '%s' '([^']+)' \| base64 -d > (\S+)`).FindAllStringSubmatch(script, -1)
			if len(files) != 9 {
				t.Fatalf("expected nine files, got %d", len(files))
			}
			for _, file := range files {
				body, err := base64.StdEncoding.DecodeString(file[1])
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasSuffix(file[2], ".py") {
					cmd := exec.Command("python3", "-c", "import ast,sys; ast.parse(sys.stdin.read())")
					cmd.Stdin = bytes.NewReader(body)
					if out, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("Python syntax %s: %v %s", file[2], err, out)
					}
				}
			}
		})
	}
}

func TestImprovementConfigValidation(t *testing.T) {
	if err := validateImprovementConfig("", ""); err != nil {
		t.Fatal(err)
	}
	if err := validateImprovementConfig("ajent-social/ajent-social", ""); err != nil {
		t.Fatal(err)
	}
	for _, repo := range []string{"https://github.com/owner/repo", "owner/repo/extra", "owner/*", "Owner/repo", "owner/.."} {
		if err := validateImprovementConfig(repo, ""); err == nil {
			t.Fatalf("accepted invalid repository %q", repo)
		}
	}
	if err := validateImprovementConfig("", "arn:aws:secretsmanager:us-west-2:123456789012:secret:token"); err == nil {
		t.Fatal("secret without repository accepted")
	}
	if err := validateImprovementConfig("owner/repo", "*"); err == nil {
		t.Fatal("wildcard secret accepted")
	}
}

func TestImprovementCredentialFailuresStayDegraded(t *testing.T) {
	config := hostConfig{Hostname: "ajent.social", Region: "us-west-2", DBHost: "db.example", DBARN: "arn:db", OwnerARN: "arn:owner", Image: "registry/app@sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64), LogGroup: "logs", Stack: "production", ImprovementRepository: "owner/repo", ImprovementGitHubSecretARN: "arn:aws:secretsmanager:us-west-2:123456789012:secret:improvement-test"}
	script, err := bootstrap(config)
	if err != nil {
		t.Fatal(err)
	}
	files := regexp.MustCompile(`printf '%s' '([^']+)' \| base64 -d > (\S+)`).FindAllStringSubmatch(script, -1)
	var program string
	for _, file := range files {
		if strings.HasSuffix(file[2], "refresh.py") {
			body, err := base64.StdEncoding.DecodeString(file[1])
			if err != nil {
				t.Fatal(err)
			}
			program = string(body)
		}
	}
	if program == "" {
		t.Fatal("refresh.py missing")
	}
	cmd := exec.Command("python3", "-c", `
import contextlib,io,json,subprocess,sys
from unittest.mock import patch
source=json.load(sys.stdin)
for token in ['scoped-test-token','bad\nINJECTED=yes','bad\rvalue','bad value','',None, {'wrong':'format'}]:
    state={}
    class Writer:
        def __enter__(self):return self
        def __exit__(self,*args):return False
        def write(self,text):state['env']+=text
    class Path:
        def __init__(self,*args):pass
        def read_text(self):return json.dumps(source['config'])
        def write_text(self,text):state['env']=text
        def open(self,mode):assert mode=='a';return Writer()
        def replace(self,target):state['promoted']=True
    def output(args,**kwargs):
        if 'get-login-password' in args:return b'registry-test-password'
        arn=args[-1]
        if arn==source['config']['improvementGitHubSecretArn']:
            assert kwargs.get('timeout')==15
            assert kwargs.get('stderr')==subprocess.DEVNULL
            if token is None:raise subprocess.CalledProcessError(1,args,stderr=b'private-provider-detail')
            return json.dumps({'SecretString':token}).encode()
        return json.dumps({'SecretString':'test-value'}).encode()
    stderr=io.StringIO()
    with patch('pathlib.Path',Path),patch('subprocess.check_output',output),patch('subprocess.run'),patch('os.umask'),contextlib.redirect_stderr(stderr):
        exec(compile(source['program'],'refresh.py','exec'),{})
    assert state['promoted'] is True
    assert 'AJENT_IMPROVEMENT_REPOSITORY=owner/repo\n' in state['env']
    assert 'AJENT_DSN=' in state['env']
    assert ('AJENT_IMPROVEMENT_GITHUB_TOKEN=' in state['env'])==(token=='scoped-test-token')
    assert 'INJECTED=' not in state['env']
    assert 'private-provider-detail' not in stderr.getvalue()
print('seven credential cases passed')
`)
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var configMap map[string]any
	if err := json.Unmarshal(configJSON, &configMap); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"config": configMap, "program": program})
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if !strings.Contains(string(out), "seven credential cases passed") {
		t.Fatalf("unexpected output: %s", out)
	}
}
