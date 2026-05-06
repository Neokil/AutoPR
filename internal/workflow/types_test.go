package workflow //nolint:testpackage // needs access to unexported loadEmbeddedDefault

import (
	"strings"
	"testing"
)

// Happy path: a valid two-state workflow with all supported action types.
func TestValidate_valid(t *testing.T) {
	t.Parallel()
	cfg := Config{
		States: []StateConfig{
			{
				Name:   "step-one",
				Prompt: "prompts/step-one.md",
				Actions: []ActionConfig{
					{Label: "Approve", Type: ActionMoveToState, Target: "step-two"},
					{Label: "Feedback", Type: ActionProvideFeedback},
				},
			},
			{
				Name:   "step-two",
				Prompt: "prompts/step-two.md",
				Actions: []ActionConfig{
					{
						Label:    "Run Checks",
						Type:     ActionRunScript,
						Commands: []string{"npm test"},
						OnSuccess: &ActionConfig{
							Label: "Accept", Type: ActionMoveToState, Target: "done",
						},
						OnFailure: &ActionConfig{
							Label: "Fix", Type: ActionProvideFeedback,
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

// done / cancelled / failed are implicitly valid targets even though they are
// not declared as states.
func TestValidate_terminalTargets(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"done", "cancelled", "failed"} {
		cfg := Config{
			States: []StateConfig{
				{
					Name:   "step",
					Prompt: "p.md",
					Actions: []ActionConfig{
						{Label: "End", Type: ActionMoveToState, Target: target},
					},
				},
			},
		}
		err := cfg.Validate()
		if err != nil {
			t.Errorf("target %q should be valid terminal, got error: %v", target, err)
		}
	}
}

// A transition to a state that doesn't exist is the most common real-world
// workflow mistake and must be caught at load time.
func TestValidate_unknownTarget(t *testing.T) {
	t.Parallel()
	cfg := Config{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{Label: "Go", Type: ActionMoveToState, Target: "nonexistent"},
				},
			},
		},
	}
	if cfg.Validate() == nil {
		t.Fatal("expected error for move_to_state with unknown target")
	}
}

// A run_script action without any handler leaves the workflow stuck.
func TestValidate_runScriptNoHandlers(t *testing.T) {
	t.Parallel()
	cfg := Config{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{Label: "Run", Type: ActionRunScript, Commands: []string{"echo hi"}},
				},
			},
		},
	}
	if cfg.Validate() == nil {
		t.Fatal("expected error for run_script without any handler")
	}
}

// The embedded default workflow must always be valid and non-empty.
func TestEmbeddedDefaultIsValid(t *testing.T) {
	t.Parallel()
	cfg, err := loadEmbeddedDefault()
	if err != nil {
		t.Fatalf("embedded default workflow is invalid: %v", err)
	}
	if len(cfg.States) == 0 {
		t.Fatal("embedded default workflow has no states")
	}
	first, _ := cfg.FirstState()
	if !strings.Contains(first.Prompt, "fetch-ticket") {
		t.Errorf("expected first state to be fetch-ticket, got prompt %q", first.Prompt)
	}
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"done", "cancelled", "failed"} {
		if !IsTerminal(name) {
			t.Errorf("expected %q to be terminal", name)
		}
	}
	if IsTerminal("investigation") {
		t.Error("expected non-terminal state not to be terminal")
	}
}

func TestFirstState(t *testing.T) {
	t.Parallel()
	cfg := Config{
		States: []StateConfig{
			{Name: "first", Prompt: "f.md"},
			{Name: "second", Prompt: "s.md"},
		},
	}
	s, ok := cfg.FirstState()
	if !ok || s.Name != "first" {
		t.Fatalf("expected first state, got ok=%v name=%q", ok, s.Name)
	}
	if _, ok = (Config{}).FirstState(); ok {
		t.Fatal("expected FirstState to return false on empty config")
	}
}

func TestStateByName(t *testing.T) {
	t.Parallel()
	cfg := Config{
		States: []StateConfig{
			{Name: "alpha", Prompt: "a.md"},
			{Name: "beta", Prompt: "b.md"},
		},
	}
	s, ok := cfg.StateByName("beta")
	if !ok || s.Name != "beta" {
		t.Fatalf("expected beta, got ok=%v name=%q", ok, s.Name)
	}
	if _, ok = cfg.StateByName("gamma"); ok {
		t.Fatal("expected false for unknown state")
	}
}
