package providers

import "fmt"

type PromptRenderer struct {
	promptsDir string
}

func NewPromptRenderer(promptsDir string) (*PromptRenderer, error) {
	if err := initializePromptTemplates(promptsDir); err != nil {
		return nil, err
	}
	return &PromptRenderer{promptsDir: promptsDir}, nil
}

func (r *PromptRenderer) Render(name string, data any) (string, error) {
	if r == nil {
		return "", fmt.Errorf("prompt renderer is nil")
	}
	return renderPromptTemplate(r.promptsDir, name, data)
}
