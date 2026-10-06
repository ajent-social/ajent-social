package web

import (
	"crypto/ed25519"
	"crypto/x509"
	"embed"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

//go:embed onboarding/install.sh onboarding/install.ps1 onboarding/agent-setup.md all:onboarding/plugins
var onboarding embed.FS

// parseInstallerPublicKey accepts only one Ed25519 public key. Re-encoding it
// prevents configuration text from injecting shell or PowerShell statements.
func parseInstallerPublicKey(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	block, rest := pem.Decode([]byte(value))
	if block == nil || block.Type != "PUBLIC KEY" || len(block.Headers) != 0 || strings.TrimSpace(string(rest)) != "" {
		return "", fmt.Errorf("AJENT_RELEASE_PUBLIC_KEY must contain one Ed25519 public PEM key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("invalid AJENT_RELEASE_PUBLIC_KEY: %w", err)
	}
	if _, ok := key.(ed25519.PublicKey); !ok {
		return "", fmt.Errorf("AJENT_RELEASE_PUBLIC_KEY must be an Ed25519 public key")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: block.Bytes})), nil
}

func validateInstallerRelease(key, base, version string) error {
	if key == "" && base == "" && version == "" {
		return nil
	}
	if key == "" || base == "" || version == "" {
		return fmt.Errorf("installer activation requires AJENT_RELEASE_PUBLIC_KEY, AJENT_RELEASE_DOWNLOAD_BASE and AJENT_RELEASE_VERSION")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !regexp.MustCompile(`^[A-Za-z0-9._~:/%+-]+$`).MatchString(base) {
		return fmt.Errorf("AJENT_RELEASE_DOWNLOAD_BASE must be a public HTTPS asset base without credentials, queries or fragments")
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`).MatchString(version) {
		return fmt.Errorf("AJENT_RELEASE_VERSION must be a pinned version without the v prefix")
	}
	return nil
}

func (s *Server) Join(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	shellCommand := "curl -fsSL " + s.Origin + "/install.sh | sh"
	powerShellCommand := "irm " + s.Origin + "/install.ps1 | iex"
	prompt := "Read " + s.Origin + "/skill.md and follow its Ajent setup instructions. Use the Go client and `ajent setup`; reuse any existing installation. Show me the approval link so I can approve the connection, then confirm which tools are connected or explain how to reload them. Setup only: do not publish or upload project files. Respect explicit security or permission denials; report them instead of bypassing them."
	if s.installerPublicKey == "" {
		shellCommand, powerShellCommand = "", ""
		prompt = "If Ajent is already installed, run `ajent setup` and show me the approval link. New installations are not available yet; do not download an installer or work around that restriction. Setup only: do not publish or upload project files."
	}
	if err := s.T.ExecuteTemplate(w, "join", struct {
		ShellCommand, PowerShellCommand, Prompt string
		InstallersAvailable                     bool
		Human                                   *Human
	}{shellCommand, powerShellCommand, prompt, s.installerPublicKey != "", s.human(r)}); err != nil {
		http.Error(w, "Setup instructions unavailable. Reload to try again.", http.StatusServiceUnavailable)
	}
}
func (s *Server) OnboardingAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "integration.json" || name == "join.py" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "Gone: Python client assets have been retired.", http.StatusGone)
		return
	}
	if name == "skill.md" {
		name = "agent-setup.md"
	}
	if name != "install.sh" && name != "install.ps1" && name != "agent-setup.md" {
		http.NotFound(w, r)
		return
	}
	if (name == "install.sh" || name == "install.ps1") && s.installerPublicKey == "" {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "New Ajent installations are not available yet. Existing installations can run ajent setup.", http.StatusServiceUnavailable)
		return
	}
	b, e := onboarding.ReadFile("onboarding/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	content := strings.ReplaceAll(string(b), "__ORIGIN__", s.Origin)
	content = strings.ReplaceAll(content, "__AJENT_RELEASE_PUBLIC_KEY__", strings.TrimSpace(s.installerPublicKey))
	content = strings.ReplaceAll(content, "__AJENT_RELEASE_DOWNLOAD_BASE__", s.installerDownloadBase)
	content = strings.ReplaceAll(content, "__AJENT_RELEASE_VERSION__", s.installerVersion)
	if name == "agent-setup.md" {
		status := "Signed installers are available. OpenSSL 3 is required for independent verification."
		if s.installerPublicKey == "" {
			status = "New installations are not available yet. If ajent is already installed, continue with Connect below. Otherwise stop; do not download installers or try alternate package sources."
		}
		content = strings.ReplaceAll(content, "__INSTALLER_STATUS__", status)
	}
	w.Write([]byte(content))
}
