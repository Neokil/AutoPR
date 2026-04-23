package workflow

import "fmt"

type ActionType string

const (
	ActionProvideFeedback ActionType = "provide_feedback"
	ActionMoveToState     ActionType = "move_to_state"
	ActionRunScript       ActionType = "run_script"
)

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
			return ErrStateEmptyName
		}
		if state.Prompt == "" {
			return fmt.Errorf("state %q: %w", state.Name, ErrStateEmptyPrompt)
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
		return ErrActionEmptyLabel
	}
	switch a.Type {
	case ActionProvideFeedback:
		if len(a.Commands) > 0 {
			return ErrProvideFeedbackNoCommands
		}
		if a.OnSuccess != nil || a.OnFailure != nil || a.Always != nil {
			return ErrProvideFeedbackNoHandlers
		}
		if a.Target != "" {
			return ErrProvideFeedbackNoTarget
		}
	case ActionMoveToState:
		if a.Target == "" {
			return ErrMoveToStateRequiresTarget
		}
		if !stateNames[a.Target] && !IsTerminal(a.Target) {
			return fmt.Errorf("target %q: %w", a.Target, ErrInvalidStateTarget)
		}
		if len(a.Commands) > 0 {
			return ErrMoveToStateNoCommands
		}
		if a.OnSuccess != nil || a.OnFailure != nil || a.Always != nil {
			return ErrMoveToStateNoHandlers
		}
	case ActionRunScript:
		if len(a.Commands) == 0 {
			return ErrRunScriptRequiresCommands
		}
		if a.OnSuccess == nil && a.OnFailure == nil && a.Always == nil {
			return ErrRunScriptRequiresHandler
		}
		if a.Target != "" {
			return ErrRunScriptNoTarget
		}
		for _, sub := range []*ActionConfig{a.OnSuccess, a.OnFailure, a.Always} {
			if sub == nil {
				continue
			}
			if sub.Type == ActionRunScript {
				return ErrNestedRunScript
			}
			if err := validateHandler(*sub, stateNames); err != nil {
				return fmt.Errorf("handler: %w", err)
			}
		}
	default:
		return fmt.Errorf("action type %q: %w", a.Type, ErrUnknownActionType)
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
	switch name {
	case "done", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func (s StateConfig) TimelineLabel() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Name
}
