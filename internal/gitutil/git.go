package gitutil

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Neokil/AutoPR/internal/shell"
)

func RepoRoot(ctx context.Context, cwd string) (string, error) {
	res, err := shell.Run(ctx, cwd, nil, "", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func CurrentBranch(ctx context.Context, repoRoot string) (string, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func OriginURL(ctx context.Context, repoRoot string) (string, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "remote", "get-url", "origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func DefaultBranch(ctx context.Context, repoRoot string) (string, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(res.Stdout)
	branch = strings.TrimPrefix(branch, "origin/")
	return branch, nil
}

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
		return "", fmt.Errorf("origin remote is not a supported GitHub URL: %s", origin)
	}
}

func WorktreeAdd(ctx context.Context, repoRoot, branch, worktreePath, baseBranch string) error {
	if baseBranch == "" {
		baseBranch = "HEAD"
	}
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "worktree", "add", "-B", branch, worktreePath, baseBranch)
	if err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	return nil
}

func WorktreePath(repoRoot, stateDirName, ticketNumber string) string {
	return filepath.Join(repoRoot, stateDirName, "worktrees", ticketNumber)
}

func WorktreeRemove(ctx context.Context, repoRoot, worktreePath string) error {
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "worktree", "remove", worktreePath, "--force")
	return err
}

func PushBranch(ctx context.Context, repoRoot, branch string) error {
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "push", "-u", "origin", branch)
	return err
}

func CreatePR(ctx context.Context, repoRoot, title, bodyFile, base string) (string, error) {
	args := []string{"pr", "create", "--draft", "--assignee", "@me", "--title", title, "--body-file", bodyFile}
	if base != "" {
		args = append(args, "--base", base)
	}
	res, err := shell.Run(ctx, repoRoot, nil, "", "gh", args...)
	if err != nil {
		msg := strings.TrimSpace(res.Stderr)
		if msg != "" {
			return "", fmt.Errorf("%w\nstderr: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

func AheadCount(ctx context.Context, repoRoot, baseRef string) (int, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "rev-list", "--count", baseRef+"..HEAD")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(res.Stdout))
	if err != nil {
		return 0, fmt.Errorf("parse ahead count: %w", err)
	}
	return n, nil
}

func HasChanges(ctx context.Context, repoRoot string) (bool, error) {
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(res.Stdout) != "", nil
}

func CommitAll(ctx context.Context, repoRoot, message string) error {
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "add", "-A")
	if err != nil {
		return err
	}
	_, err = shell.Run(ctx, repoRoot, nil, "", "git", "commit", "-m", message)
	return err
}
