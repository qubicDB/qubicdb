package core

import (
	"fmt"
	"net/url"
	"strings"
)

// ---------------------------------------------------------------------------
// Connection String Parser
// ---------------------------------------------------------------------------
//
// QubicDB connection strings follow a URI-style format inspired by other
// databases (MongoDB, Redis, PostgreSQL):
//
//   qubicdb://[user:password@]host1[:port1][,host2[:port2]...][/indexID]
//
// Examples:
//   qubicdb://localhost:6060
//   qubicdb://admin:secret@localhost:6060
//   qubicdb://admin:secret@localhost:6060/my-index-uuid
//   qubicdb://admin:secret@node1:6060,node2:6060/my-index-uuid
//
// The scheme "qubicdb" is required. TLS connections use "qubicdb+tls".
// Multiple hosts (comma-separated) are supported for future multi-node
// deployments; the current implementation returns all hosts for the caller
// to implement routing/sticky-session logic.

// ConnInfo holds parsed connection string components.
type ConnInfo struct {
	// Scheme is the protocol scheme ("qubicdb" or "qubicdb+tls").
	Scheme string

	// User is the authentication username (empty if not provided).
	User string

	// Password is the authentication password (empty if not provided).
	Password string

	// Hosts is a list of host:port pairs. At least one is always present.
	Hosts []string

	// IndexID is the optional default index (path segment after the first slash).
	IndexID string

	// TLS is true when the scheme is "qubicdb+tls".
	TLS bool
}

// ParseConnString parses a QubicDB connection string.
//
//   qubicdb://[user:password@]host1[:port1][,host2[:port2]...][/indexID]
//   qubicdb+tls://[user:password@]host1[:port1][,host2[:port2]...][/indexID]
//
// Returns an error if the scheme is invalid or no hosts are found.
func ParseConnString(raw string) (*ConnInfo, error) {
	if raw == "" {
		return nil, fmt.Errorf("connection string must not be empty")
	}

	// Validate scheme
	if !strings.HasPrefix(raw, "qubicdb://") && !strings.HasPrefix(raw, "qubicdb+tls://") {
		return nil, fmt.Errorf("connection string must start with qubicdb:// or qubicdb+tls://, got: %s", raw)
	}

	info := &ConnInfo{}

	// Determine TLS from scheme
	if strings.HasPrefix(raw, "qubicdb+tls://") {
		info.Scheme = "qubicdb+tls"
		info.TLS = true
	} else {
		info.Scheme = "qubicdb"
	}

	// Replace scheme with http:// so net/url can parse it
	normalized := strings.Replace(raw, info.Scheme+"://", "http://", 1)

	// Handle comma-separated hosts: net/url doesn't support them natively.
	// We temporarily replace commas in the host portion.
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid connection string: %w", err)
	}

	// Extract user info
	if parsed.User != nil {
		info.User = parsed.User.Username()
		info.Password, _ = parsed.User.Password()
	}

	// Extract hosts — the Host field may contain commas for multi-node
	hostPart := parsed.Host
	if hostPart == "" {
		return nil, fmt.Errorf("connection string must contain at least one host")
	}

	hosts := strings.Split(hostPart, ",")
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		// Add default port if missing
		if !strings.Contains(h, ":") {
			h += ":6060"
		}
		info.Hosts = append(info.Hosts, h)
	}

	if len(info.Hosts) == 0 {
		return nil, fmt.Errorf("connection string must contain at least one host")
	}

	// Extract index ID from path
	path := strings.TrimPrefix(parsed.Path, "/")
	if path != "" {
		info.IndexID = path
	}

	return info, nil
}

// String reconstructs the connection string (password masked).
func (c *ConnInfo) String() string {
	var sb strings.Builder
	sb.WriteString(c.Scheme)
	sb.WriteString("://")

	if c.User != "" {
		sb.WriteString(c.User)
		if c.Password != "" {
			sb.WriteString(":***")
		}
		sb.WriteByte('@')
	}

	sb.WriteString(strings.Join(c.Hosts, ","))

	if c.IndexID != "" {
		sb.WriteByte('/')
		sb.WriteString(c.IndexID)
	}

	return sb.String()
}

// PrimaryHost returns the first host in the list.
func (c *ConnInfo) PrimaryHost() string {
	if len(c.Hosts) == 0 {
		return ""
	}
	return c.Hosts[0]
}

// BaseURL returns the HTTP(S) base URL for the primary host.
func (c *ConnInfo) BaseURL() string {
	scheme := "http"
	if c.TLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.PrimaryHost())
}
