package gitutil

import (
	"context"
	"fmt"
	"path/filepath"
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

func CreatePR(ctx context.Context, repoRoot, title, bodyFile, base string) (string, error) {
	args := []string{"pr", "create", "--title", title, "--body-file", bodyFile}
	if base != "" {
		args = append(args, "--base", base)
	}
	res, err := shell.Run(ctx, repoRoot, nil, "", "gh", args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}
