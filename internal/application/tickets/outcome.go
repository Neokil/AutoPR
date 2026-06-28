package tickets

// RunOutcome carries the decision-relevant result of running a workflow state.
// It is returned alongside (not inside) the error so callers can inspect it even
// when the error is non-nil. The zero value means "no signal to report".
type RunOutcome struct {
	Provider     string
	QuotaReached bool
}
