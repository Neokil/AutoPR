package gitutil

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"ai-ticket-worker/internal/shell"
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

func CreatePR(ctx context.Context, repoRoot, title, bodyFile, base string) (string, error) {
	args := []string{"pr", "create", "--title", title, "--body-file", bodyFile}
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
	res, err := shell.Run(ctx, repoRoot, nil, "", "git", "rev-list", "--count", fmt.Sprintf("%s..HEAD", baseRef))
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
	if _, err := shell.Run(ctx, repoRoot, nil, "", "git", "add", "-A"); err != nil {
		return err
	}
	_, err := shell.Run(ctx, repoRoot, nil, "", "git", "commit", "-m", message)
	return err
}
