package promptresolver

import (
	"errors"
	"fmt"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/ydapi"
	"log/slog"
	"strings"
)

const (
	STATUS_COMPLETED   = "completed"
	STATUS_QUEUED      = "queued"
	STATUS_IN_PROGRESS = "in_progress"
	STATUS_FAILED      = "failed"
	STATUS_CANCELLED   = "cancelled"
	STATUS_INCOMPLETE  = "incomplete"
)

type YdPromptRequest struct {
	Model           string  `json:"model"`
	Background      bool    `json:"background"`
	Input           string  `json:"input"`
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int64   `json:"max_output_tokens"`
}

// OutputItem - элемент массива output
type OutputItem struct {
	Content []ContentItem `json:"content"`
}

// ContentItem - элемент массива content
type ContentItem struct {
	Text string `json:"text"`
}

type YdOperationResponse struct {
	ID     string       `json:"id"`
	Error  *string      `json:"error"`
	Status *string      `json:"status"`
	Output []OutputItem `json:"output"`
}

type YdPr struct {
	logger        *slog.Logger
	promptManager *promptmanager.PromptManager
	ydApi         *ydapi.YdApi
}

var _ PromptResolver = (*YdPr)(nil)

func NewYdPr(promptManager *promptmanager.PromptManager, ydApi *ydapi.YdApi, logger *slog.Logger) *YdPr {
	return &YdPr{
		logger:        logger,
		promptManager: promptManager,
		ydApi:         ydApi,
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

	url := fmt.Sprintf("https://ai.api.cloud.yandex.net/v1/responses/%s", operationID)

	var response YdOperationResponse
	err := ydpr.ydApi.Request(ydapi.GET, url, 200, nil, &response)
	if err != nil {
		ydpr.logger.Error("Error when gemma prompt", "error", err.Error())
		return ydpr.errorResult(errors.New("error when prompt generate"))
	}

	return ydpr.responseProcessing(response, errors.New("error when operation check"))
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

	request := YdPromptRequest{
		Model:           fmt.Sprintf("gpt://%s/yandexgpt-5-pro/latest", ydpr.ydApi.GetFolderId()),
		Input:           "сгенерируй промпт для генерации изображения. Максимум 500 символов",
		Temperature:     0.6,
		MaxOutputTokens: 1000,
		Background:      true,
	}

	var response YdOperationResponse
	err := ydpr.ydApi.Request(ydapi.POST, "https://ai.api.cloud.yandex.net/v1/responses", 200, request, &response)
	if err != nil {
		ydpr.logger.Error("Error when gemma prompt", "error", err.Error())
		return ydpr.errorResult(errors.New("error when prompt generate"))
	}

	return ydpr.responseProcessing(response, errors.New("error when prompt generate"))

}

func (ydpr *YdPr) responseProcessing(response YdOperationResponse, errorText error) (PromptResolveResult, error) {

	if response.Error != nil {
		ydpr.logger.Error("Prompt generate return error", "error", response.Error)
		return ydpr.errorResult(errorText)
	}

	if response.Status == nil {
		ydpr.logger.Error("Status can not be null")
		return ydpr.errorResult(errorText)
	}

	status := *response.Status

	switch status {
	case STATUS_COMPLETED:
		text := response.GetOutputText()
		if text == "" {
			ydpr.logger.Error("Generated prompt is empty")
			return ydpr.errorResult(errorText)
		}

		ydpr.logger.Debug("Generated prompt", "text", text)
		return PromptResolveResult{
			Prompt: &promptmanager.PromptValue{
				Prompt:  text,
				IsGemma: false,
			},
			IsPending: false,
		}, nil
	case STATUS_QUEUED, STATUS_IN_PROGRESS:
		ydpr.logger.Info("Status prompt generated operation", "status", status)

		if response.ID == "" {
			ydpr.logger.Error("Id can not be null")
			return ydpr.errorResult(errorText)
		}

		return PromptResolveResult{
			OperationID: response.ID,
			IsPending:   true,
		}, nil
	case STATUS_FAILED, STATUS_CANCELLED, STATUS_INCOMPLETE:
		ydpr.logger.Error("Prompt generation operation failed", "status", status)
		return ydpr.errorResult(fmt.Errorf("%w: terminal operation status %q", errorText, status))
	default:
		ydpr.logger.Error("Unknown prompt generation operation status", "status", status)
		return ydpr.errorResult(fmt.Errorf("%w: unknown operation status %q", errorText, status))
	}
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

func (r YdOperationResponse) GetOutputText() string {
	for _, output := range r.Output {
		if len(output.Content) > 0 {
			return output.Content[0].Text
		}
	}
	return ""
}
