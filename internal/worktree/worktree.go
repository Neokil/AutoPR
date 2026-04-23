package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Neokil/AutoPR/internal/gitutil"
)

func Ensure(ctx context.Context, repoRoot, stateDirName, ticketNumber, branchName, baseBranch string) (string, error) {
	path := gitutil.WorktreePath(repoRoot, stateDirName, ticketNumber)
	_, err := os.Stat(path)
	if err == nil {
		return path, nil
	}
	err = os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		return "", fmt.Errorf("prepare worktree parent: %w", err)
	}
	err = gitutil.WorktreeAdd(ctx, repoRoot, branchName, path, baseBranch)
	if err != nil {
		return "", fmt.Errorf("create worktree: %w", err)
	}
	return path, nil
}
