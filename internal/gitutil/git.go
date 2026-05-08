package gitutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Neokil/AutoPR/internal/shell"
)

// RepoRoot returns the absolute path to the root of the git repository containing cwd.
func RepoRoot(ctx context.Context, cwd string) (string, error) {
	res, err := shell.Run(ctx, cwd, nil, "", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

// CurrentBranch returns the name of the currently checked-out branch in repoRoot.
func CurrentBranch(ctx context.Context, repoRoot string) (string, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

// OriginURL returns the URL of the origin remote for the repository at repoRoot.
func OriginURL(ctx context.Context, repoRoot string) (string, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("git remote get-url: %w", err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

// DefaultBranch returns the default branch name as tracked by origin/HEAD (e.g. "main").
func DefaultBranch(ctx context.Context, repoRoot string) (string, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", fmt.Errorf("git symbolic-ref: %w", err)
	}
	branch := strings.TrimSpace(res.Stdout)
	branch = strings.TrimPrefix(branch, "origin/")

	return branch, nil
}

// GitHubBlobBase returns the GitHub blob URL prefix for browsing files on baseBranch
// (e.g. "https://github.com/owner/repo/blob/main"). Falls back to "main" if baseBranch is empty.
func GitHubBlobBase(ctx context.Context, repoRoot, baseBranch string) (string, error) {
	origin, err := OriginURL(ctx, repoRoot)
	if err != nil {
		return "", err
	}
	ownerRepo, err := parseGitHubOwnerRepo(origin)
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(baseBranch)
	if branch == "" {
		branch, err = DefaultBranch(ctx, repoRoot)
		if err != nil || strings.TrimSpace(branch) == "" {
			branch = "main"
		}
	}

	return fmt.Sprintf("https://github.com/%s/blob/%s", ownerRepo, branch), nil
}

func parseGitHubOwnerRepo(origin string) (string, error) {
	origin = strings.TrimSpace(origin)
	origin = strings.TrimSuffix(origin, ".git")
	switch {
	case strings.HasPrefix(origin, "git@github.com:"):
		return strings.TrimPrefix(origin, "git@github.com:"), nil
	case strings.HasPrefix(origin, "ssh://git@github.com/"):
		return strings.TrimPrefix(origin, "ssh://git@github.com/"), nil
	case strings.HasPrefix(origin, "https://github.com/"):
		return strings.TrimPrefix(origin, "https://github.com/"), nil
	case strings.HasPrefix(origin, "http://github.com/"):
		return strings.TrimPrefix(origin, "http://github.com/"), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedGitHubURL, origin)
	}
}

// WorktreeAdd creates a new git worktree at worktreePath on branch, forked from baseBranch.
func WorktreeAdd(ctx context.Context, repoRoot, branch, worktreePath, baseBranch string) error {
	if baseBranch == "" {
		def, err := DefaultBranch(ctx, repoRoot)
		if err != nil || strings.TrimSpace(def) == "" {
			baseBranch = "main"
		} else {
			baseBranch = def
		}
	}
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "worktree", "add", "-B", branch, worktreePath, baseBranch)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}

	return nil
}

// WorktreePath returns the conventional filesystem path for a ticket's worktree.
func WorktreePath(repoRoot, stateDirName, ticketNumber string) string {
	return filepath.Join(repoRoot, stateDirName, "worktrees", ticketNumber)
}

// EnsureWorktree returns the ticket worktree path, creating it if needed.
func EnsureWorktree(ctx context.Context, repoRoot, stateDirName, ticketNumber, branchName, baseBranch string) (string, error) {
	path := WorktreePath(repoRoot, stateDirName, ticketNumber)
	_, err := os.Stat(path)
	if err == nil {
		return path, nil
	}
	err = os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return "", fmt.Errorf("prepare worktree parent: %w", err)
	}
	err = WorktreeAdd(ctx, repoRoot, branchName, path, baseBranch)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}

	return path, nil
}

// RenameBranch renames the currently checked-out branch inside worktreePath.
func RenameBranch(ctx context.Context, worktreePath, newName string) error {
	_, err := shell.Run(ctx, worktreePath, nil, "", "git", "branch", "-m", newName)
	if err != nil {
		return fmt.Errorf("git branch -m: %w", err)
	}

	return nil
}

// WorktreeRemove force-removes the worktree at worktreePath from the repository.
func WorktreeRemove(ctx context.Context, repoRoot, worktreePath string) error {
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "worktree", "remove", worktreePath, "--force")
	if err != nil {
		return fmt.Errorf("git worktree remove: %w", err)
	}

	return nil
}

// PushBranch pushes branch to origin and sets the upstream tracking reference.
func PushBranch(ctx context.Context, repoRoot, branch string) error {
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "push", "-u", "origin", branch)
	if err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	return nil
}

// CreatePR opens a draft pull request via `gh` and returns its URL.
func CreatePR(ctx context.Context, repoRoot, title, bodyFile, base string) (string, error) {
	args := []string{"pr", "create", "--draft", "--assignee", "@me", "--title", title, "--body-file", bodyFile}
	if base != "" {
		args = append(args, "--base", base)
	}
	res, err := shell.Run(ctx, repoRoot, nil, "", "gh", args...)
	if err != nil {
		msg := strings.TrimSpace(res.Stderr)
		if msg != "" {
			return "", fmt.Errorf("gh pr create: %w\nstderr: %s", err, msg)
		}

		return "", fmt.Errorf("gh pr create: %w", err)
	}

	return strings.TrimSpace(res.Stdout), nil
}

// AheadCount returns the number of commits on HEAD that are not in baseRef.
func AheadCount(ctx context.Context, repoRoot, baseRef string) (int, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "rev-list", "--count", baseRef+"..HEAD")
	if err != nil {
		return 0, fmt.Errorf("git rev-list: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(res.Stdout))
	if err != nil {
		return 0, fmt.Errorf("parse ahead count: %w", err)
	}

	return n, nil
}

// HasChanges reports whether the working tree has any uncommitted changes.
func HasChanges(ctx context.Context, repoRoot string) (bool, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}

	return strings.TrimSpace(res.Stdout) != "", nil
}

// CommitAll stages all changes and creates a commit with the given message.
func CommitAll(ctx context.Context, repoRoot, message string) error {
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "add", "-A")
	if err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	_, err = shell.Run(ctx, repoRoot, nil, "", "git", "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}
