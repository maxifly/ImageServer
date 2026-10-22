package promptresolver

import (
	"errors"
	"fmt"
	"imgserver/internal/pkg/promptmanager"
	"log/slog"
	"strings"
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

	return ydpr.errorResult(errors.New("invalid prompt request type"))

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
	if strings.TrimSpace(promptId) == "" {
		err := errors.New("prompt id cannot be empty")
		ydpr.logger.Error("Invalid prompt id", "error", err)
		return ydpr.errorResult(err)
	}

	prompt, ok := ydpr.promptManager.GetPromptByCode(promptId)
	if !ok {
		ydpr.logger.Error("Cannot find prompt", "id", promptId)
		return ydpr.errorResult(errors.New("cannot find prompt"))
	}

	promptValue := ydpr.promptManager.GetPromptValue(prompt)
	return ydpr.promptRaw(&promptValue)

}

func (ydpr *YdPr) promptRandom() (PromptResolveResult, error) {
	prompt, err := ydpr.promptManager.GetRandomPromptValue()
	if err != nil {
		ydpr.logger.Error("Error when get prompt", "error", err.Error())

		return ydpr.promptDefault()
	}

	return ydpr.promptRaw(&prompt)
}

func (ydpr *YdPr) promptRaw(prompt *promptmanager.PromptValue) (PromptResolveResult, error) {
	if prompt == nil {
		return ydpr.errorResult(errors.New("prompt cannot be nil"))
	}
	if strings.TrimSpace(prompt.Prompt) == "" {
		return ydpr.errorResult(errors.New("prompt text cannot be empty"))
	}
	if prompt.IsGemma {
		return ydpr.promptGemma(prompt)
	}
	return PromptResolveResult{
		Prompt:    prompt,
		IsPending: false,
	}, nil
}

func (ydpr *YdPr) promptGemma(prompt *promptmanager.PromptValue) (PromptResolveResult, error) {
	return ydpr.errorResult(fmt.Errorf("not supported"))
}

func (ydpr *YdPr) promptDefault() (PromptResolveResult, error) {
	ydpr.logger.Debug("Return default prompt", "prompt", "test")
	return PromptResolveResult{
		Prompt: &promptmanager.PromptValue{
			Prompt:  "test",
			IsGemma: false,
		},
		IsPending: false,
	}, nil
}

func (ydpr *YdPr) errorResult(errorV error) (PromptResolveResult, error) {
	return PromptResolveResult{
		Prompt:    nil,
		IsPending: false,
	}, errorV
}
