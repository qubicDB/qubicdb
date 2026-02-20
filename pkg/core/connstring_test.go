package core

import (
	"testing"
)

func TestParseConnString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantHosts []string
		wantUser  string
		wantPass  string
		wantIndex string
		wantTLS   bool
	}{
		{
			name:      "simple host",
			input:     "qubicdb://localhost:6060",
			wantHosts: []string{"localhost:6060"},
		},
		{
			name:      "host without port gets default",
			input:     "qubicdb://localhost",
			wantHosts: []string{"localhost:6060"},
		},
		{
			name:      "with credentials",
			input:     "qubicdb://admin:secret@localhost:6060",
			wantHosts: []string{"localhost:6060"},
			wantUser:  "admin",
			wantPass:  "secret",
		},
		{
			name:      "with index ID",
			input:     "qubicdb://admin:secret@localhost:6060/my-index",
			wantHosts: []string{"localhost:6060"},
			wantUser:  "admin",
			wantPass:  "secret",
			wantIndex: "my-index",
		},
		{
			name:      "multi-host",
			input:     "qubicdb://admin:pass@node1:6060,node2:6061/my-index",
			wantHosts: []string{"node1:6060", "node2:6061"},
			wantUser:  "admin",
			wantPass:  "pass",
			wantIndex: "my-index",
		},
		{
			name:      "TLS scheme",
			input:     "qubicdb+tls://admin:pass@localhost:6060",
			wantHosts: []string{"localhost:6060"},
			wantUser:  "admin",
			wantPass:  "pass",
			wantTLS:   true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			input:   "mongodb://localhost:6060",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseConnString(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(info.Hosts) != len(tt.wantHosts) {
				t.Fatalf("hosts: got %v, want %v", info.Hosts, tt.wantHosts)
			}
			for i, h := range info.Hosts {
				if h != tt.wantHosts[i] {
					t.Errorf("host[%d]: got %q, want %q", i, h, tt.wantHosts[i])
				}
			}
			if info.User != tt.wantUser {
				t.Errorf("user: got %q, want %q", info.User, tt.wantUser)
			}
			if info.Password != tt.wantPass {
				t.Errorf("password: got %q, want %q", info.Password, tt.wantPass)
			}
			if info.IndexID != tt.wantIndex {
				t.Errorf("indexID: got %q, want %q", info.IndexID, tt.wantIndex)
			}
			if info.TLS != tt.wantTLS {
				t.Errorf("tls: got %v, want %v", info.TLS, tt.wantTLS)
			}
		})
	}
}

func TestConnInfoString(t *testing.T) {
	info := &ConnInfo{
		Scheme:   "qubicdb",
		User:     "admin",
		Password: "secret",
		Hosts:    []string{"localhost:6060"},
		IndexID:  "my-index",
	}

	s := info.String()
	expected := "qubicdb://admin:***@localhost:6060/my-index"
	if s != expected {
		t.Errorf("String(): got %q, want %q", s, expected)
	}
}

func TestConnInfoBaseURL(t *testing.T) {
	info := &ConnInfo{
		Scheme: "qubicdb",
		Hosts:  []string{"localhost:6060"},
	}
	if info.BaseURL() != "http://localhost:6060" {
		t.Errorf("BaseURL: got %q", info.BaseURL())
	}

	info.TLS = true
	info.Scheme = "qubicdb+tls"
	if info.BaseURL() != "https://localhost:6060" {
		t.Errorf("BaseURL TLS: got %q", info.BaseURL())
	}
}
