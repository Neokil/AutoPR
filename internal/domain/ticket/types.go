// Package ticket defines tracker-facing ticket entities.
package ticket

// Ticket is a structured representation of a project ticket fetched from an external tracker.
type Ticket struct {
	Number             string            `json:"number"`
	ID                 string            `json:"id"`
	Title              string            `json:"title"`
	Description        string            `json:"description"`
	AcceptanceCriteria string            `json:"acceptance_criteria"`
	Priority           string            `json:"priority"`
	URL                string            `json:"url"`
	Labels             []string          `json:"labels,omitempty"`
	WorkflowFields     map[string]string `json:"workflow_fields,omitempty"`
	ParentTicket       *Context          `json:"parent_ticket,omitempty"`
	Epic               *Context          `json:"epic,omitempty"`
}

// Context is a lightweight reference to a parent ticket or epic.
type Context struct {
	ID          string `json:"id"`
	Number      string `json:"number,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}
