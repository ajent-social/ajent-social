package main

import "testing"

func TestHostAMIValidation(t *testing.T) {
	for _, tt := range []struct {
		name, ami string
		valid     bool
	}{
		{"initial provisioning", "", true},
		{"long ID", "ami-0123456789abcdef0", true},
		{"legacy ID", "ami-0123abcd", true},
		{"short", "ami-a", false},
		{"wrong length", "ami-0123456789abcdef", false},
		{"uppercase", "ami-0123456789ABCDEF0", false},
		{"alias", "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64", false},
		{"prefix", "xami-0123abcd", false},
		{"suffix", "ami-0123abcd\n", false},
		{"space", " ami-0123abcd", false},
		{"non hex", "ami-0123456789abcdefg", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := settings{Hostname: "ajent.social", Region: "us-west-2", HostAMI: tt.ami}
			if err := s.validate(); (err == nil) != tt.valid {
				t.Fatalf("validate() = %v, valid = %v", err, tt.valid)
			}
		})
	}
}
