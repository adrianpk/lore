package lore

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"
)

// NormalizeRepositoryRemote returns a credential-free canonical repository URL.
func NormalizeRepositoryRemote(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("normalize repository remote: remote is required")
	}

	if !strings.Contains(raw, "://") {
		converted, ok := convertSCPLikeRemote(raw)
		if !ok {
			return "", fmt.Errorf("normalize repository remote: unsupported remote syntax")
		}

		raw = converted
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("normalize repository remote: %w", err)
	}

	return normalizeRepositoryURL(parsed)
}

func normalizeRepositoryURL(parsed *url.URL) (string, error) {
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "ssh", "git", "http", "https":
	default:
		return "", fmt.Errorf("normalize repository remote: unsupported scheme %q", parsed.Scheme)
	}

	if parsed.Hostname() == "" {
		return "", fmt.Errorf("normalize repository remote: host is required")
	}

	parsed.Host = canonicalHost(parsed.Hostname(), parsed.Port())
	parsed.User = nil

	repositoryPath := path.Clean("/" + strings.TrimLeft(parsed.Path, "/"))
	if repositoryPath == "/" || repositoryPath == "/." {
		return "", fmt.Errorf("normalize repository remote: repository path is required")
	}

	if !strings.HasSuffix(repositoryPath, ".git") {
		repositoryPath += ".git"
	}

	parsed.Path = repositoryPath
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""

	return parsed.String(), nil
}

func convertSCPLikeRemote(raw string) (string, bool) {
	separator := strings.IndexByte(raw, ':')
	if separator <= 0 || separator == len(raw)-1 {
		return "", false
	}

	authority := raw[:separator]
	repositoryPath := raw[separator+1:]

	if strings.ContainsAny(authority, "/\\") || strings.HasPrefix(repositoryPath, "//") {
		return "", false
	}

	return "ssh://" + authority + "/" + strings.TrimLeft(repositoryPath, "/"), true
}

func canonicalHost(host, port string) string {
	host = strings.ToLower(host)
	if port != "" {
		return net.JoinHostPort(host, port)
	}

	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}

	return host
}
