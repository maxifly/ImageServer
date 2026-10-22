package promptresolver

import "imgserver/internal/pkg/promptmanager"

type PromptResolveResult struct {
	Prompt      *promptmanager.PromptValue
	OperationID string
	IsPending   bool
}

type PromptRequestType string

const (
	PromptRequestRandom PromptRequestType = "random"
	PromptRequestByID   PromptRequestType = "by_id"
	PromptRequestRaw    PromptRequestType = "raw"
)

type PromptRequest struct {
	Type     PromptRequestType
	PromptID string
	Prompt   *promptmanager.PromptValue
}

type PromptResolver interface {
	Resolve(request PromptRequest) (PromptResolveResult, error)
	CheckOperation(operationID string) (PromptResolveResult, error)
}
