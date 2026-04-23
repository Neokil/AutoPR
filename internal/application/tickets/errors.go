package tickets

import "errors"

var (
	ErrTicketRunning       = errors.New("ticket is already running")
	ErrTicketNotWaiting    = errors.New("ticket is not waiting for an action")
	ErrStateNotFound       = errors.New("state not found in workflow")
	ErrActionNotFound      = errors.New("action not found in state")
	ErrTargetStateRequired = errors.New("target state is required")
	ErrTargetNotFound      = errors.New("target state not found in workflow")
	ErrFeedbackRequired    = errors.New("feedback message is required")
	ErrUnknownActionType   = errors.New("unknown action type")
	ErrScriptSubAction     = errors.New("scripts cannot be used as sub-actions")
	ErrUnsupportedSubAction = errors.New("unsupported sub-action type")
	ErrWorkflowNoStates     = errors.New("workflow has no states defined")
)
