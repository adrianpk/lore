package lore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitInspectorReadsRepositoryEvidence(t *testing.T) {
	t.Parallel()

	root := initializeTestRepository(t)
	runTestGit(t, root, "remote", "add", "origin", "https://user:token@example.test/team/example.git")

	statusBefore := testGitOutput(t, root, "status", "--porcelain=v1", "--untracked-files=normal")

	evidence, err := NewGitInspector().Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	assertRepositoryEvidence(t, evidence.Repository, root)
	assertGitEvidence(t, evidence.Git)

	statusAfter := testGitOutput(t, root, "status", "--porcelain=v1", "--untracked-files=normal")
	if statusAfter != statusBefore {
		t.Fatalf("Inspect() changed Git status from %q to %q", statusBefore, statusAfter)
	}
}

func assertRepositoryEvidence(t *testing.T, repository RepositoryReference, root string) {
	t.Helper()

	if repository.Name != filepath.Base(root) || repository.Root != root {
		t.Fatalf("repository = %#v", repository)
	}

	wantRemote := "https://example.test/team/example.git"
	if repository.Remote != wantRemote || repository.Identity != wantRemote {
		t.Fatalf("repository identity = %#v", repository)
	}
}

func assertGitEvidence(t *testing.T, reference GitReference) {
	t.Helper()

	if reference.Branch != "dev" || reference.Subject != "feat: add test milestone" {
		t.Fatalf("Git evidence = %#v", reference)
	}

	wantAuthoredAt := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)

	wantCommittedAt := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	if !reference.AuthoredAt.Equal(wantAuthoredAt) || !reference.CommittedAt.Equal(wantCommittedAt) {
		t.Fatalf("Git times = %s, %s", reference.AuthoredAt, reference.CommittedAt)
	}

	if !reference.WorktreeClean {
		t.Fatal("worktree is not clean")
	}
}

func TestGitInspectorReportsDirtyWorktree(t *testing.T) {
	t.Parallel()

	root := initializeTestRepository(t)
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("local state\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	evidence, err := NewGitInspector().Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if evidence.Git.WorktreeClean {
		t.Fatal("worktree is clean")
	}
}

func TestGitInspectorSupportsDetachedHeadAndLocalIdentity(t *testing.T) {
	t.Parallel()

	root := initializeTestRepository(t)
	runTestGit(t, root, "checkout", "--detach")

	evidence, err := NewGitInspector().Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if evidence.Git.Branch != "" {
		t.Fatalf("branch = %q, want empty", evidence.Git.Branch)
	}

	if !strings.HasPrefix(evidence.Repository.Identity, "git-root:") || evidence.Repository.Remote != "" {
		t.Fatalf("repository = %#v", evidence.Repository)
	}
}

func TestGitInspectorFallsBackFromUnusableRemote(t *testing.T) {
	t.Parallel()

	root := initializeTestRepository(t)
	runTestGit(t, root, "remote", "add", "origin", "/workspace/upstream")

	evidence, err := NewGitInspector().Inspect(context.Background(), root)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if evidence.Repository.Remote != "" || !strings.HasPrefix(evidence.Repository.Identity, "git-root:") {
		t.Fatalf("repository = %#v", evidence.Repository)
	}
}

func TestGitInspectorRejectsNonRepository(t *testing.T) {
	t.Parallel()

	if _, err := NewGitInspector().Inspect(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected repository error")
	}
}

func initializeTestRepository(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	runTestGit(t, root, "init", "--initial-branch=dev")

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Example\n"), 0o600); err != nil {
		t.Fatalf("write repository file: %v", err)
	}

	runTestGit(t, root, "add", "README.md")

	command := exec.Command(
		"git",
		"-C", root,
		"-c", "user.name=Lore Test",
		"-c", "user.email=lore@example.test",
		"commit", "-m", "feat: add test milestone",
	)

	command.Env = append(
		os.Environ(),
		"GIT_AUTHOR_DATE=2026-07-01T10:00:00+02:00",
		"GIT_COMMITTER_DATE=2026-07-01T11:30:00+02:00",
	)

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("commit test repository: %v: %s", err, output)
	}

	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	return canonical
}

func runTestGit(t *testing.T, root string, arguments ...string) {
	t.Helper()

	commandArguments := append([]string{"-C", root}, arguments...)
	command := exec.Command("git", commandArguments...)

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run test Git command: %v: %s", err, output)
	}
}

func testGitOutput(t *testing.T, root string, arguments ...string) string {
	t.Helper()

	commandArguments := append([]string{"-C", root}, arguments...)

	output, err := exec.Command("git", commandArguments...).Output()
	if err != nil {
		t.Fatalf("read test Git output: %v", err)
	}

	return string(output)
}
