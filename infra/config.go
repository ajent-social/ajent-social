package main

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type settings struct {
	CanonicalHostname, CertificateARN                                                                   string
	Hostname, Region, Stack, ImageDigest, CaddyImage, OpenrouterSecretARN, AlertEmail, CloudflareZoneID string
	RollingService, RetireLegacyHost                                                                    bool
}

func loadSettings(ctx *pulumi.Context) (settings, error) {
	c := config.New(ctx, "")
	hostname, err := c.Try("hostname")
	if err != nil {
		if !errors.Is(err, config.ErrMissingVar) {
			return settings{}, err
		}
		hostname = "ajent.social"
	}
	s := settings{CanonicalHostname: c.Get("canonicalHostname"), CertificateARN: c.Get("certificateArn"), Hostname: hostname, Region: config.Get(ctx, "aws:region"), Stack: ctx.Stack(), ImageDigest: c.Get("imageDigest"), CaddyImage: c.Get("caddyImage"), OpenrouterSecretARN: c.Get("openrouterSecretArn"), AlertEmail: c.Get("alertEmail"), CloudflareZoneID: c.Get("cloudflareZoneId")}
	for key, dst := range map[string]*bool{"rollingService": &s.RollingService, "retireLegacyHost": &s.RetireLegacyHost} {
		value, err := c.Try(key)
		if errors.Is(err, config.ErrMissingVar) {
			continue
		}
		if err != nil {
			return s, fmt.Errorf("reading %s: %w", key, err)
		}
		if value != "true" && value != "false" {
			return s, fmt.Errorf("%s must be true or false", key)
		}
		*dst = value == "true"
	}
	return s, s.validate()
}
func (s settings) validate() error {
	if s.CanonicalHostname != "" && s.CanonicalHostname != "ajent.social" {
		return fmt.Errorf("canonicalHostname must be ajent.social for this cutover")
	}
	if !regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`).MatchString(s.Hostname) {
		return fmt.Errorf("hostname must be a lowercase DNS name")
	}
	if s.RetireLegacyHost && !s.RollingService {
		return fmt.Errorf("retireLegacyHost requires rollingService")
	}
	if s.ImageDigest != "" && !regexp.MustCompile(`^sha256:[a-f0-9]{64}$`).MatchString(s.ImageDigest) {
		return fmt.Errorf("imageDigest must be sha256:<64 hex characters>")
	}
	if s.ImageDigest != "" && !regexp.MustCompile(`^caddy@sha256:[a-f0-9]{64}$`).MatchString(s.CaddyImage) {
		return fmt.Errorf("set caddyImage to caddy@sha256:<digest> before enabling the host")
	}
	if s.Region == "" {
		return fmt.Errorf("set aws:region explicitly")
	}
	return nil
}
