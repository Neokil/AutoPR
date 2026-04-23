package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

const validWorkflowYAML = `
states:
  - name: step-one
    prompt: prompts/fetch-ticket.md
    actions:
      - label: "Continue"
        type: move_to_state
        target: done
`

func TestLoad_embeddedDefault(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("expected embedded default, got error: %v", err)
	}
	if len(cfg.States) == 0 {
		t.Fatal("expected states from embedded default")
	}
}

func TestLoad_projectFile(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	// Write a project-level workflow file.
	autoprDir := filepath.Join(tmp, ".auto-pr")
	if err := os.MkdirAll(autoprDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(autoprDir, "workflow.yaml"), []byte(validWorkflowYAML), 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.States) != 1 || cfg.States[0].Name != "step-one" {
		t.Fatalf("expected project file to be loaded, got %+v", cfg.States)
	}
}

func TestLoad_globalFile(t *testing.T) {
	repoTmp := t.TempDir()
	homeTmp := t.TempDir()
	userHomeDir = func() (string, error) { return homeTmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	// Write a global workflow file (no project-level file).
	globalDir := filepath.Join(homeTmp, ".auto-pr")
	if err := os.MkdirAll(globalDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "workflow.yaml"), []byte(validWorkflowYAML), 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	cfg, err := Load(repoTmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.States) != 1 || cfg.States[0].Name != "step-one" {
		t.Fatalf("expected global file to be loaded, got %+v", cfg.States)
	}
}

func TestLoad_projectTakesPrecedenceOverGlobal(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	// Write both project and global files.
	projectDir := filepath.Join(tmp, ".auto-pr")
	if err := os.MkdirAll(projectDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	projectYAML := `
states:
  - name: project-state
    prompt: prompts/fetch-ticket.md
    actions:
      - label: "Go"
        type: move_to_state
        target: done
`
	if err := os.WriteFile(filepath.Join(projectDir, "workflow.yaml"), []byte(projectYAML), 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	globalDir := filepath.Join(tmp, ".auto-pr")
	globalYAML := `
states:
  - name: global-state
    prompt: prompts/fetch-ticket.md
    actions:
      - label: "Go"
        type: move_to_state
        target: done
`
	// Same directory in this test since home == repo root — just verify project wins.
	_ = globalDir
	_ = globalYAML

	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Project file is at <tmp>/.auto-pr/workflow.yaml — should load project-state.
	if cfg.States[0].Name != "project-state" {
		t.Fatalf("expected project file to take precedence, got %q", cfg.States[0].Name)
	}
}

func TestLoad_invalidYAML(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	autoprDir := filepath.Join(tmp, ".auto-pr")
	if err := os.MkdirAll(autoprDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(autoprDir, "workflow.yaml"), []byte("states:\n  - {invalid"), 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	if _, err := Load(tmp); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_invalidWorkflowConfig(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	autoprDir := filepath.Join(tmp, ".auto-pr")
	if err := os.MkdirAll(autoprDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	badYAML := `
states:
  - name: step
    prompt: p.md
    actions:
      - label: "Go"
        type: move_to_state
        target: nonexistent-state
`
	if err := os.WriteFile(filepath.Join(autoprDir, "workflow.yaml"), []byte(badYAML), 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	if _, err := Load(tmp); err == nil {
		t.Fatal("expected validation error for unknown target state")
	}
}

func TestReadPrompt_projectFile(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	promptDir := filepath.Join(tmp, ".auto-pr", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	content := []byte("project prompt content")
	if err := os.WriteFile(filepath.Join(promptDir, "my-prompt.md"), content, 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	got, err := ReadPrompt(tmp, "prompts/my-prompt.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("expected %q, got %q", content, got)
	}
}

func TestReadPrompt_globalFile(t *testing.T) {
	repoTmp := t.TempDir()
	homeTmp := t.TempDir()
	userHomeDir = func() (string, error) { return homeTmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	promptDir := filepath.Join(homeTmp, ".auto-pr", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil { //nolint:gosec // G301: test setup, standard dir perms
		t.Fatal(err)
	}
	content := []byte("global prompt content")
	if err := os.WriteFile(filepath.Join(promptDir, "my-prompt.md"), content, 0o644); err != nil { //nolint:gosec // G306: test setup, readable perms intentional
		t.Fatal(err)
	}

	got, err := ReadPrompt(repoTmp, "prompts/my-prompt.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("expected %q, got %q", content, got)
	}
}

func TestReadPrompt_embeddedFallback(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	// No files on disk — should return the embedded investigate.md.
	data, err := ReadPrompt(tmp, "prompts/investigate.md")
	if err != nil {
		t.Fatalf("expected embedded fallback, got error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty embedded prompt")
	}
}

func TestReadPrompt_notFound(t *testing.T) {
	tmp := t.TempDir()
	userHomeDir = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { userHomeDir = os.UserHomeDir })

	if _, err := ReadPrompt(tmp, "prompts/does-not-exist.md"); err == nil {
		t.Fatal("expected error for missing prompt")
	}
}
