package promptresolver

import (
	"errors"
	"fmt"
	"imgserver/internal/pkg/promptmanager"
	"log/slog"
)

type YdPr struct {
	logger        *slog.Logger
	promptManager *promptmanager.PromptManager
}

var _ PromptResolver = (*YdPr)(nil)

func NewYdPr(promptManager *promptmanager.PromptManager, logger *slog.Logger) *YdPr {
	return &YdPr{
		logger:        logger,
		promptManager: promptManager,
	}
}

func (ydpr *YdPr) Resolve(request PromptRequest) (PromptResolveResult, error) {
	if request.Type == PromptRequestRandom {
		return ydpr.promptRandom()
	} else if request.Type == PromptRequestByID {
		return ydpr.promptById(request.PromptID)
	} else if request.Type == PromptRequestRaw {
		return ydpr.promptRaw(request.Prompt)
	}

	return PromptResolveResult{
		Prompt:      request.Prompt,
		IsPending:   false,
		OperationID: "",
	}, errors.New("invalid prompt request type")

}

func (ydpr *YdPr) CheckOperation(operationID string) (PromptResolveResult, error) {
	//TODO make
	return PromptResolveResult{
		Prompt: &promptmanager.PromptValue{
			Prompt:  "test",
			IsGemma: false,
		},
		OperationID: operationID,
		IsPending:   false,
	}, nil
}

func (ydpr *YdPr) promptById(promptId string) (PromptResolveResult, error) {

}

func (ydpr *YdPr) promptRandom() (PromptResolveResult, error) {

}

func (ydpr *YdPr) promptRaw(prompt *promptmanager.PromptValue) (PromptResolveResult, error) {
	//TODO make
	if prompt.IsGemma {
		return ydpr.promptGemma(prompt)
	}
	return PromptResolveResult{
		prompt,
		"",
		false,
	}, nil
}

func (ydpr *YdPr) promptGemma(prompt *promptmanager.PromptValue) (PromptResolveResult, error) {
	return PromptResolveResult{
		nil,
		"123",
		false,
	}, fmt.Errorf("not supported")
}
