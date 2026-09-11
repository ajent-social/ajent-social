package main

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// Field order matches the original TypeScript JSON so migration does not roll hosts.
type hostConfig struct {
	CanonicalHostname            string `json:"canonicalHostname,omitempty"`
	LegacyOrigin                 string `json:"legacyOrigin,omitempty"`
	OpenrouterSecretARN          string `json:"openrouterSecretArn,omitempty"`
	ImprovementRepository        string `json:"improvementRepository,omitempty"`
	ImprovementGitHubSecretARN   string `json:"improvementGitHubSecretArn,omitempty"`
	Hostname                     string `json:"hostname"`
	Region                       string `json:"region"`
	DBHost                       string `json:"dbHost"`
	DBARN                        string `json:"dbArn"`
	OwnerARN                     string `json:"ownerArn"`
	Image                        string `json:"image"`
	CaddyImage                   string `json:"caddyImage"`
	LogGroup                     string `json:"logGroup"`
	Stack                        string `json:"stack"`
	BehindLoadBalancer           bool   `json:"behindLoadBalancer,omitempty"`
}

//go:embed bootstrap/*.tmpl
var bootstrapFiles embed.FS

var (
	improvementRepositoryPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,38}/[a-z0-9_.-]{1,100}$`)
	improvementSecretARNPattern  = regexp.MustCompile(`^arn:aws(?:-us-gov|-cn)?:secretsmanager:[a-z0-9-]+:[0-9]{12}:secret:[a-zA-Z0-9/_+=.@-]+$`)
)

// Provision only a dedicated repository credential; never infer a local gh token.
func validateImprovementConfig(repo, secretARN string) error {
	if repo != "" {
		parts := strings.Split(repo, "/")
		if !improvementRepositoryPattern.MatchString(repo) || len(parts) != 2 || parts[1] == "." || parts[1] == ".." {
			return fmt.Errorf("improvementRepository must be a lowercase owner/repository")
		}
	}
	if secretARN != "" {
		if repo == "" {
			return fmt.Errorf("improvementGitHubSecretArn requires improvementRepository")
		}
		if !improvementSecretARNPattern.MatchString(secretARN) {
			return fmt.Errorf("improvementGitHubSecretArn must identify one Secrets Manager secret")
		}
	}
	return nil
}

func bootstrap(c hostConfig) (string, error) {
	if err := validateImprovementConfig(c.ImprovementRepository, c.ImprovementGitHubSecretARN); err != nil {
		return "", err
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(c); err != nil {
		return "", fmt.Errorf("encoding host config: %w", err)
	}
	var out strings.Builder
	out.WriteString("#!/bin/bash\nset -euo pipefail\numask 077\ndnf install -y docker awscli-2 python3\nsystemctl enable --now docker\ninstall -d -m 755 /opt/ajent\ninstall -d -m 700 /var/lib/ajent/caddy-data /var/lib/ajent/caddy-config\n")
	write := func(path, body string) {
		fmt.Fprintf(&out, "printf '%%s' '%s' | base64 -d > %s\n", base64.StdEncoding.EncodeToString([]byte(body)), path)
	}
	write("/opt/ajent/config.json", strings.TrimSuffix(encoded.String(), "\n"))
	render := func(name, path string) error {
		raw, err := bootstrapFiles.ReadFile("bootstrap/" + name + ".tmpl")
		if err != nil {
			return fmt.Errorf("reading bootstrap %s: %w", name, err)
		}
		t, err := template.New(name).Option("missingkey=error").Parse(string(raw))
		if err != nil {
			return fmt.Errorf("parsing bootstrap %s: %w", name, err)
		}
		var body bytes.Buffer
		if err := t.Execute(&body, c); err != nil {
			return fmt.Errorf("rendering bootstrap %s: %w", name, err)
		}
		write(path+name, body.String())
		return nil
	}
	for _, name := range []string{"Caddyfile", "refresh.py", "run.py", "metrics.py"} {
		if err := render(name, "/opt/ajent/"); err != nil {
			return "", err
		}
	}
	out.WriteString("chmod 644 /opt/ajent/Caddyfile\ncurl --fail --retry 5 --proto =https --tlsv1.2 https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem -o /opt/ajent/rds.pem\nchmod 644 /opt/ajent/rds.pem\n")
	for _, name := range []string{"ajent-app.service", "ajent-proxy.service", "ajent-metrics.service", "ajent-metrics.timer"} {
		if err := render(name, "/etc/systemd/system/"); err != nil {
			return "", err
		}
	}
	out.WriteString("systemctl daemon-reload\nsystemctl enable --now ajent-app ")
	if !c.BehindLoadBalancer {
		out.WriteString("ajent-proxy ")
	}
	out.WriteString("ajent-metrics.timer\n")
	return out.String(), nil
}
