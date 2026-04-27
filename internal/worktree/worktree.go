// Package worktree manages git worktree creation for tickets.
package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Neokil/AutoPR/internal/gitutil"
)

// Ensure returns the worktree path for the ticket, creating the worktree if it does not exist.
func Ensure(ctx context.Context, repoRoot, stateDirName, ticketNumber, branchName, baseBranch string) (string, error) {
	path := gitutil.WorktreePath(repoRoot, stateDirName, ticketNumber)
	_, err := os.Stat(path)
	if err == nil {
		return path, nil
	}
	err = os.MkdirAll(filepath.Dir(path), 0o755) //nolint:gosec,mnd // G301: 0755 correct for project directories
	if err != nil {
		return "", fmt.Errorf("prepare worktree parent: %w", err)
	}
	err = gitutil.WorktreeAdd(ctx, repoRoot, branchName, path, baseBranch)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}

	return path, nil
}
