package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Neokil/AutoPR/internal/application/orchestrator"
	"github.com/Neokil/AutoPR/internal/contracts/api"
	"github.com/Neokil/AutoPR/internal/gitutil"
	"github.com/Neokil/AutoPR/internal/providers"
	"github.com/Neokil/AutoPR/internal/state"
)

func (s *server) repoRuntimeFromBody(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	var req api.RepoRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")

		return "", "", false
	}
	if strings.TrimSpace(req.RepoPath) == "" {
		writeError(w, http.StatusBadRequest, "repo_path is required")

		return "", "", false
	}
	root, id, _, err := s.runtimeForRepoPath(r.Context(), req.RepoPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())

		return "", "", false
	}

	return root, id, true
}

func (s *server) runtimeForRepoPath(ctx context.Context, repoPath string) (string, string, *repoRuntime, error) {
	repoRoot, err := resolveRepoRoot(ctx, repoPath)
	if err != nil {
		return "", "", nil, err
	}
	repoRec, err := s.meta.UpsertRepo(repoRoot)
	if err != nil {
		return "", "", nil, err
	}
	rt, err := s.runtimeForRepo(repoRoot)
	if err != nil {
		return "", "", nil, err
	}

	return repoRoot, repoRec.ID, rt, nil
}

func (s *server) runtimeForRepo(repoRoot string) (*repoRuntime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rt, ok := s.runtimes[repoRoot]; ok {
		return rt, nil
	}
	provider, err := providers.NewFromConfig(s.cfg)
	if err != nil {
		return nil, err
	}
	rt := &repoRuntime{
		svc:      orchestrator.NewWorkflowService(s.cfg, repoRoot, provider),
		repoRoot: repoRoot,
		store:    state.NewStore(repoRoot, s.cfg.StateDirName),
	}
	s.runtimes[repoRoot] = rt

	return rt, nil
}

func resolveRepoRoot(ctx context.Context, repoPath string) (string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return "", errRepoPathEmpty
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("resolve repo_path: %w", err)
	}
	dir := absPath
	info, err := os.Stat(absPath) //nolint:gosec // G703: absPath is the resolved canonical repo path
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(absPath)
	}
	root, err := gitutil.RepoRoot(ctx, dir)
	if err != nil {
		return "", fmt.Errorf("repo_path is not a git repository: %w", err)
	}

	return filepath.Clean(root), nil
}

func discoverRepositoriesFromConfig(entries []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, raw := range entries {
		root := strings.TrimSpace(raw)
		if root == "" {
			continue
		}
		root = expandHome(root)
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if isGitRepositoryDir(absRoot) {
			if _, ok := seen[absRoot]; !ok {
				seen[absRoot] = struct{}{}
				out = append(out, absRoot)
			}

			continue
		}
		children, err := os.ReadDir(absRoot)
		if err != nil {
			continue
		}
		for _, child := range children {
			if !child.IsDir() {
				continue
			}
			candidate := filepath.Join(absRoot, child.Name())
			if !isGitRepositoryDir(candidate) {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	sort.Strings(out)

	return out
}

func expandHome(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}

		return path
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, rest)
		}
	}

	return path
}

func isGitRepositoryDir(path string) bool {
	st, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}

	return st.IsDir() || st.Mode().IsRegular()
}
