package workflow

import "fmt"

// ActionType identifies the behaviour of a workflow action.
type ActionType string

// Known action types used in workflow YAML.
const (
	ActionProvideFeedback ActionType = "provide_feedback"
	ActionMoveToState     ActionType = "move_to_state"
	ActionRunScript       ActionType = "run_script"
)

// Config is the top-level structure parsed from a workflow.yaml file.
type Config struct {
	States []StateConfig `yaml:"states"`
}

// StateConfig defines a single step in a workflow: the prompt to run and the actions available afterward.
type StateConfig struct {
	Name               string         `yaml:"name"`
	DisplayName        string         `yaml:"display_name,omitempty"`
	Prompt             string         `yaml:"prompt"`
	PrimaryArtifact    string         `yaml:"primary_artifact,omitempty"`
	PrePromptCommands  []string       `yaml:"pre_prompt_commands"`
	PostPromptCommands []string       `yaml:"post_prompt_commands"`
	Actions            []ActionConfig `yaml:"actions"`
}

// ActionConfig describes what happens when a human (or script) triggers an action on a waiting ticket.
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
func (c Config) Validate() error {
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
			err := validateAction(action, stateNames)
			if err != nil {
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

func validateActionNode(action ActionConfig, stateNames map[string]bool, requireLabel bool) error {
	if requireLabel && action.Label == "" {
		return ErrActionEmptyLabel
	}
	switch action.Type {
	case ActionProvideFeedback:
		if len(action.Commands) > 0 {
			return ErrProvideFeedbackNoCommands
		}
		if action.OnSuccess != nil || action.OnFailure != nil || action.Always != nil {
			return ErrProvideFeedbackNoHandlers
		}
		if action.Target != "" {
			return ErrProvideFeedbackNoTarget
		}
	case ActionMoveToState:
		if action.Target == "" {
			return ErrMoveToStateRequiresTarget
		}
		if !stateNames[action.Target] && !IsTerminal(action.Target) {
			return fmt.Errorf("target %q: %w", action.Target, ErrInvalidStateTarget)
		}
		if len(action.Commands) > 0 {
			return ErrMoveToStateNoCommands
		}
		if action.OnSuccess != nil || action.OnFailure != nil || action.Always != nil {
			return ErrMoveToStateNoHandlers
		}
	case ActionRunScript:
		if len(action.Commands) == 0 {
			return ErrRunScriptRequiresCommands
		}
		if action.OnSuccess == nil && action.OnFailure == nil && action.Always == nil {
			return ErrRunScriptRequiresHandler
		}
		if action.Target != "" {
			return ErrRunScriptNoTarget
		}
		for _, sub := range []*ActionConfig{action.OnSuccess, action.OnFailure, action.Always} {
			if sub == nil {
				continue
			}
			if sub.Type == ActionRunScript {
				return ErrNestedRunScript
			}
			err := validateHandler(*sub, stateNames)
			if err != nil {
				return fmt.Errorf("handler: %w", err)
			}
		}
	default:
		return fmt.Errorf("action type %q: %w", action.Type, ErrUnknownActionType)
	}

	return nil
}

// StateByName returns the StateConfig with the given name, if it exists.
func (c Config) StateByName(name string) (StateConfig, bool) {
	for _, s := range c.States {
		if s.Name == name {
			return s, true
		}
	}

	return StateConfig{}, false
}

// FirstState returns the first state defined in the workflow.
func (c Config) FirstState() (StateConfig, bool) {
	if len(c.States) == 0 {
		return StateConfig{}, false
	}

	return c.States[0], true
}

// Terminal state name constants.
const (
	TerminalDone      = "done"
	TerminalCancelled = "cancelled"
	TerminalFailed    = "failed"
)

// IsTerminal reports whether name is a built-in terminal state name.
func IsTerminal(name string) bool {
	switch name {
	case TerminalDone, TerminalCancelled, TerminalFailed:
		return true
	default:
		return false
	}
}

// TimelineLabel returns the display name shown in the UI timeline, falling back to Name.
func (s StateConfig) TimelineLabel() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}

	return s.Name
}
