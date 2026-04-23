package workflow

import "errors"

var (
	ErrStateEmptyName              = errors.New("state has empty name")
	ErrStateEmptyPrompt            = errors.New("state has empty prompt")
	ErrActionEmptyLabel            = errors.New("action has empty label")
	ErrProvideFeedbackNoCommands   = errors.New("provide_feedback must not have commands")
	ErrProvideFeedbackNoHandlers   = errors.New("provide_feedback must not have script handlers")
	ErrProvideFeedbackNoTarget     = errors.New("provide_feedback must not have a target")
	ErrMoveToStateRequiresTarget   = errors.New("move_to_state requires a target")
	ErrInvalidStateTarget          = errors.New("target is not a defined state or known terminal name (done, cancelled, failed)")
	ErrMoveToStateNoCommands       = errors.New("move_to_state must not have commands")
	ErrMoveToStateNoHandlers       = errors.New("move_to_state must not have script handlers")
	ErrRunScriptRequiresCommands   = errors.New("run_script requires at least one command")
	ErrRunScriptRequiresHandler    = errors.New("run_script requires at least one handler (on_success, on_failure, or always)")
	ErrRunScriptNoTarget           = errors.New("run_script must not have a target; use on_success/on_failure/always instead")
	ErrNestedRunScript             = errors.New("nested run_script is not supported; add multiple commands to the commands list")
	ErrUnknownActionType           = errors.New("unknown action type")
	ErrPromptNotFound              = errors.New("prompt not found in project, global config, or embedded defaults")
)
