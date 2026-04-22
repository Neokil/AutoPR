package workflow

import "fmt"

type ActionType string

const (
	ActionProvideFeedback ActionType = "provide_feedback"
	ActionMoveToState     ActionType = "move_to_state"
	ActionRunScript       ActionType = "run_script"
)

var terminalStateNames = map[string]bool{
	"done":      true,
	"cancelled": true,
	"failed":    true,
}

type WorkflowConfig struct {
	States []StateConfig `yaml:"states"`
}

type StateConfig struct {
	Name               string         `yaml:"name"`
	DisplayName        string         `yaml:"display_name,omitempty"`
	Prompt             string         `yaml:"prompt"`
	PrimaryArtifact    string         `yaml:"primary_artifact,omitempty"`
	PrePromptCommands  []string       `yaml:"pre_prompt_commands"`
	PostPromptCommands []string       `yaml:"post_prompt_commands"`
	Actions            []ActionConfig `yaml:"actions"`
}

type ActionConfig struct {
	Label string     `yaml:"label"`
	Type  ActionType `yaml:"type"`

	// move_to_state
	Target string `yaml:"target,omitempty"`

	// run_script
	Commands  []string      `yaml:"commands,omitempty"`
	OnSuccess *ActionConfig `yaml:"on_success,omitempty"`
	OnFailure *ActionConfig `yaml:"on_failure,omitempty"`
	Always    *ActionConfig `yaml:"always,omitempty"`
}

// Validate checks that the config is self-consistent.
func (c WorkflowConfig) Validate() error {
	stateNames := make(map[string]bool, len(c.States))
	for _, s := range c.States {
		stateNames[s.Name] = true
	}
	for _, state := range c.States {
		if state.Name == "" {
			return fmt.Errorf("state has empty name")
		}
		if state.Prompt == "" {
			return fmt.Errorf("state %q has empty prompt", state.Name)
		}
		for _, action := range state.Actions {
			if err := validateAction(action, stateNames); err != nil {
				return fmt.Errorf("state %q action %q: %w", state.Name, action.Label, err)
			}
		}
	}
	return nil
}

func validateAction(a ActionConfig, stateNames map[string]bool) error {
	return validateActionNode(a, stateNames, true)
}

func validateHandler(a ActionConfig, stateNames map[string]bool) error {
	return validateActionNode(a, stateNames, false)
}

func validateActionNode(a ActionConfig, stateNames map[string]bool, requireLabel bool) error {
	if requireLabel && a.Label == "" {
		return fmt.Errorf("action has empty label")
	}
	switch a.Type {
	case ActionProvideFeedback:
		if len(a.Commands) > 0 {
			return fmt.Errorf("provide_feedback must not have commands")
		}
		if a.OnSuccess != nil || a.OnFailure != nil || a.Always != nil {
			return fmt.Errorf("provide_feedback must not have script handlers")
		}
		if a.Target != "" {
			return fmt.Errorf("provide_feedback must not have a target")
		}
	case ActionMoveToState:
		if a.Target == "" {
			return fmt.Errorf("move_to_state requires a target")
		}
		if !stateNames[a.Target] && !terminalStateNames[a.Target] {
			return fmt.Errorf("target %q is not a defined state or known terminal name (done, cancelled, failed)", a.Target)
		}
		if len(a.Commands) > 0 {
			return fmt.Errorf("move_to_state must not have commands")
		}
		if a.OnSuccess != nil || a.OnFailure != nil || a.Always != nil {
			return fmt.Errorf("move_to_state must not have script handlers")
		}
	case ActionRunScript:
		if len(a.Commands) == 0 {
			return fmt.Errorf("run_script requires at least one command")
		}
		if a.OnSuccess == nil && a.OnFailure == nil && a.Always == nil {
			return fmt.Errorf("run_script requires at least one handler (on_success, on_failure, or always)")
		}
		if a.Target != "" {
			return fmt.Errorf("run_script must not have a target; use on_success/on_failure/always instead")
		}
		for _, sub := range []*ActionConfig{a.OnSuccess, a.OnFailure, a.Always} {
			if sub == nil {
				continue
			}
			if sub.Type == ActionRunScript {
				return fmt.Errorf("nested run_script is not supported; add multiple commands to the commands list")
			}
			if err := validateHandler(*sub, stateNames); err != nil {
				return fmt.Errorf("handler: %w", err)
			}
		}
	default:
		return fmt.Errorf("unknown action type %q", a.Type)
	}
	return nil
}

// StateByName returns the StateConfig with the given name, if it exists.
func (c WorkflowConfig) StateByName(name string) (StateConfig, bool) {
	for _, s := range c.States {
		if s.Name == name {
			return s, true
		}
	}
	return StateConfig{}, false
}

// FirstState returns the first state defined in the workflow.
func (c WorkflowConfig) FirstState() (StateConfig, bool) {
	if len(c.States) == 0 {
		return StateConfig{}, false
	}
	return c.States[0], true
}

// IsTerminal reports whether name is a built-in terminal state name.
func IsTerminal(name string) bool {
	return terminalStateNames[name]
}

func (s StateConfig) TimelineLabel() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Name
}
