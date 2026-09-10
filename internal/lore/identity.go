package lore

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// NewRepositoryReference creates a validated repository reference from facts
// collected by the caller.
func NewRepositoryReference(root, remote string) (RepositoryReference, error) {
	if !filepath.IsAbs(root) {
		return RepositoryReference{}, fmt.Errorf("create repository reference: root must be absolute")
	}

	root = filepath.Clean(root)

	name := filepath.Base(root)
	if name == string(filepath.Separator) || name == "." {
		return RepositoryReference{}, fmt.Errorf("create repository reference: root must identify a repository directory")
	}

	reference := RepositoryReference{Name: name, Root: root}

	if strings.TrimSpace(remote) == "" {
		return RepositoryReference{}, fmt.Errorf("create repository reference: local identity is required")
	}

	normalized, err := NormalizeRepositoryRemote(remote)
	if err != nil {
		return RepositoryReference{}, err
	}

	reference.Identity = normalized
	reference.Remote = normalized

	if err := reference.Validate(); err != nil {
		return RepositoryReference{}, err
	}

	return reference, nil
}

func (reference RepositoryReference) validateCanonicalIdentity() error {
	if reference.Name != filepath.Base(reference.Root) {
		return fmt.Errorf("validate repository: name does not match root")
	}

	if reference.Remote == "" {
		objectID, found := strings.CutPrefix(reference.Identity, "git-root:")
		if !found || !validObjectID(objectID) {
			return fmt.Errorf("validate repository: local identity must identify a root commit")
		}

		return nil
	}

	normalized, err := NormalizeRepositoryRemote(reference.Remote)
	if err != nil {
		return fmt.Errorf("validate repository remote: %w", err)
	}

	if reference.Remote != normalized || reference.Identity != normalized {
		return fmt.Errorf("validate repository: remote identity is not canonical")
	}

	return nil
}

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
	parsed.User = normalizedRepositoryUser(parsed.Scheme, parsed.User)

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

func normalizedRepositoryUser(scheme string, user *url.Userinfo) *url.Userinfo {
	if scheme != "ssh" && scheme != "git" {
		return nil
	}

	if user == nil || user.Username() == "" {
		return nil
	}

	return url.User(user.Username())
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
