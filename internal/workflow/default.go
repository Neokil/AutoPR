package workflow

import "embed"

//go:embed default_workflow.yaml
var embeddedWorkflowYAML []byte

//go:embed prompts/*.md
var embeddedPromptsFS embed.FS
