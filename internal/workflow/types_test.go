package workflow

import (
	"strings"
	"testing"
)

func TestValidate_valid(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
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
					{Label: "Accept", Type: ActionMoveToState, Target: "done"},
					{Label: "Cancel", Type: ActionMoveToState, Target: "cancelled"},
				},
			},
		},
	}
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidate_runScript(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "check",
				Prompt: "prompts/check.md",
				Actions: []ActionConfig{
					{
						Label:    "Run Checks",
						Type:     ActionRunScript,
						Commands: []string{"npm test"},
						OnSuccess: &ActionConfig{
							Label:  "Accept",
							Type:   ActionMoveToState,
							Target: "done",
						},
						OnFailure: &ActionConfig{
							Label: "Fix",
							Type:  ActionProvideFeedback,
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

func TestValidate_runScriptAlwaysHandler(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "fetch",
				Prompt: "prompts/fetch.md",
				Actions: []ActionConfig{
					{
						Label:    "Fetch",
						Type:     ActionRunScript,
						Commands: []string{"./fetch.sh"},
						Always: &ActionConfig{
							Label: "Continue",
							Type:  ActionProvideFeedback,
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

func TestValidate_emptyStateName(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{Name: "", Prompt: "prompts/x.md"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty state name")
	}
}

func TestValidate_emptyPrompt(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{Name: "step", Prompt: ""},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestValidate_emptyActionLabel(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{Label: "", Type: ActionMoveToState, Target: "done"},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty action label")
	}
}

func TestValidate_unknownActionType(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{Label: "Go", Type: "teleport"},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unknown action type")
	}
}

func TestValidate_moveToStateNoTarget(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{Label: "Go", Type: ActionMoveToState},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for move_to_state without target")
	}
}

func TestValidate_moveToStateUnknownTarget(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
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
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for move_to_state with unknown target")
	}
}

func TestValidate_moveToStateTerminalTargets(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"done", "cancelled", "failed"} {
		cfg := WorkflowConfig{
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

func TestValidate_provideFeedbackWithCommands(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{Label: "Fb", Type: ActionProvideFeedback, Commands: []string{"echo hi"}},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for provide_feedback with commands")
	}
}

func TestValidate_provideFeedbackWithHandlers(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{
						Label: "Fb",
						Type:  ActionProvideFeedback,
						Always: &ActionConfig{
							Label: "x", Type: ActionMoveToState, Target: "done",
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for provide_feedback with script handlers")
	}
}

func TestValidate_runScriptNoCommands(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{
						Label: "Run",
						Type:  ActionRunScript,
						Always: &ActionConfig{
							Label: "ok", Type: ActionMoveToState, Target: "done",
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for run_script without commands")
	}
}

func TestValidate_runScriptNoHandlers(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
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
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for run_script without any handler")
	}
}

func TestValidate_nestedRunScript(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{
						Label:    "Run",
						Type:     ActionRunScript,
						Commands: []string{"echo hi"},
						Always: &ActionConfig{
							Label:    "inner",
							Type:     ActionRunScript,
							Commands: []string{"echo nested"},
							Always: &ActionConfig{
								Label: "done", Type: ActionMoveToState, Target: "done",
							},
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for nested run_script")
	}
}

func TestValidate_runScriptWithTarget(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{
				Name:   "step",
				Prompt: "p.md",
				Actions: []ActionConfig{
					{
						Label:    "Run",
						Type:     ActionRunScript,
						Commands: []string{"echo hi"},
						Target:   "done",
						Always: &ActionConfig{
							Label: "ok", Type: ActionMoveToState, Target: "done",
						},
					},
				},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for run_script with a target field set")
	}
}

func TestStateByName(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{Name: "alpha", Prompt: "a.md"},
			{Name: "beta", Prompt: "b.md"},
		},
	}
	s, ok := cfg.StateByName("beta")
	if !ok || s.Name != "beta" {
		t.Fatalf("expected to find state beta, got ok=%v name=%q", ok, s.Name)
	}
	_, ok = cfg.StateByName("gamma")
	if ok {
		t.Fatal("expected StateByName to return false for unknown state")
	}
}

func TestFirstState(t *testing.T) {
	t.Parallel()
	cfg := WorkflowConfig{
		States: []StateConfig{
			{Name: "first", Prompt: "f.md"},
			{Name: "second", Prompt: "s.md"},
		},
	}
	s, ok := cfg.FirstState()
	if !ok || s.Name != "first" {
		t.Fatalf("expected first state, got ok=%v name=%q", ok, s.Name)
	}

	empty := WorkflowConfig{}
	_, ok = empty.FirstState()
	if ok {
		t.Fatal("expected FirstState to return false on empty config")
	}
}

func TestIsTerminal(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"done", "cancelled", "failed"} {
		if !IsTerminal(name) {
			t.Errorf("expected %q to be terminal", name)
		}
	}
	for _, name := range []string{"investigation", "implementation", ""} {
		if IsTerminal(name) {
			t.Errorf("expected %q to not be terminal", name)
		}
	}
}

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
