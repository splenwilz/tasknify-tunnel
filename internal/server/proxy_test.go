package server

import (
	"testing"
)

func TestExtractSubdomain(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		domain    string
		want      string
		wantErr   bool
	}{
		{"simple", "myapp.tasknify.com", "tasknify.com", "myapp", false},
		{"with port", "myapp.tasknify.com:443", "tasknify.com", "myapp", false},
		{"with http port", "myapp.tasknify.com:80", "tasknify.com", "myapp", false},
		{"uppercase host", "MyApp.tasknify.com", "tasknify.com", "myapp", false},
		{"nested subdomain", "deep.myapp.tasknify.com", "tasknify.com", "deep.myapp", false},
		{"no subdomain", "tasknify.com", "tasknify.com", "", true},
		{"wrong domain", "myapp.other.com", "tasknify.com", "", true},
		{"empty host", "", "tasknify.com", "", true},
		{"ip address", "192.168.1.1", "tasknify.com", "", true},
		{"null byte", "my\x00app.tasknify.com", "tasknify.com", "", true},
		{"slash in subdomain", "my/app.tasknify.com", "tasknify.com", "", true},
		{"backslash", "my\\app.tasknify.com", "tasknify.com", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractSubdomain(tt.host, tt.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractSubdomain(%q, %q) error = %v, wantErr %v", tt.host, tt.domain, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractSubdomain(%q, %q) = %q, want %q", tt.host, tt.domain, got, tt.want)
			}
		})
	}
}
