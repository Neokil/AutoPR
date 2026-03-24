package providers

import "fmt"

type PromptRenderer struct {
	promptsDir   string
	providerName string
}

func NewPromptRenderer(promptsDir, providerName string) (*PromptRenderer, error) {
	return &PromptRenderer{promptsDir: promptsDir, providerName: providerName}, nil
}

func (r *PromptRenderer) Render(name string, data any) (string, error) {
	if r == nil {
		return "", fmt.Errorf("prompt renderer is nil")
	}
	return renderPromptTemplate(r.promptsDir, r.providerName, name, data)
}
