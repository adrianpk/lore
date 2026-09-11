package lore

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

func (app *App) inferProjectContext(ctx context.Context) (Context, bool) {
	directory, err := app.workingDirectory()
	if err != nil {
		return Context{}, false
	}

	root, err := runProjectGit(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return Context{}, false
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return Context{}, false
	}

	name, err := NormalizeCanonicalName(filepath.Base(absoluteRoot))
	if err != nil {
		return Context{}, false
	}

	remote, err := runProjectGit(ctx, absoluteRoot, "config", "--get", "remote.origin.url")
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
			return Context{}, false
		}

		remote = ""
	}

	canonicalRemote, err := NormalizeRepositoryRemote(remote)
	if err != nil {
		canonicalRemote = ""
	}

	return Context{
		Type:       ContextTypeProject,
		Name:       name,
		Repository: repositoryContextIdentity(canonicalRemote),
	}, true
}

func repositoryContextIdentity(remote string) string {
	if remote == "" {
		return ""
	}

	parsed, err := url.Parse(remote)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}

	host := strings.ToLower(parsed.Hostname())
	if parsed.Port() != "" {
		host += ":" + parsed.Port()
	}

	repositoryPath := strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git")
	if repositoryPath == "" {
		return ""
	}

	return host + "/" + repositoryPath
}

func runProjectGit(ctx context.Context, directory string, arguments ...string) (string, error) {
	commandArguments := append([]string{"-C", directory}, arguments...)

	output, err := exec.CommandContext(ctx, "git", commandArguments...).Output()
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSpace(output)), nil
}
