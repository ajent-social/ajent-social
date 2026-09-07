package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

func TestBootstrapParityAndSyntax(t *testing.T) {
	for _, mode := range []string{"legacy", "rolling"} {
		t.Run(mode, func(t *testing.T) {
			c := hostConfig{Hostname: "dogfood.ajent.social", Region: "us-west-2", DBHost: "db.example", DBARN: "arn:db", OwnerARN: "arn:owner", Image: "registry/app@sha256:" + strings.Repeat("a", 64), CaddyImage: "caddy@sha256:" + strings.Repeat("b", 64), LogGroup: "logs", Stack: "dogfood"}
			if mode == "rolling" {
				c.BehindLoadBalancer = true
				c.OpenrouterSecretARN = "arn:chat-test"
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
