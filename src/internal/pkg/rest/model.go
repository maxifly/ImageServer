package rest

import (
	"imgserver/internal/pkg/opermanager"
)

type ImageResultResponse struct {
	Image string `json:"image"`
}

// Response структура для отправки данных в формате JSON
type ImageResponse struct {
	Id     string              `json:"id"`
	Status opermanager.Status  `json:"status"`
	Error  *ErrorAttributes    `json:"error,omitempty"`
	Result ImageResultResponse `json:"response,omitempty"`
}

// StatusResponse структура для отображения статуса

type AlertMessage struct {
	Message string `json:"alertMessage"`
}

type MetricGroup struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	ErrorCount  int64   `json:"errorCount"`
	TotalCount  int64   `json:"totalCount"`
	SuccessRate float64 `json:"successRate"`
	ErrorRate   float64 `json:"errorRate"`
}

type FileAmount struct {
	DirType string `json:"dirType"`
	Amount  int64  `json:"amount"`
}

type StatusResponse struct {
	AlertMessages  []AlertMessage `json:"alerts"`
	Groups         []MetricGroup  `json:"groups"`
	ProviderGroups []MetricGroup  `json:"providerGroups"`
	FileAmounts    []FileAmount   `json:"fileAmounts"`

	YandexToday     int64 `json:"yandex_today"`
	YandexYesterday int64 `json:"yandex_yesterday"`
}

// Error структура для ошибок
type ErrorAttributes struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	DevMessage string `json:"dev_message,omitempty"`
}

// StartRequest структура для входящего запроса
type StartRequest struct {
	Type     string `json:"type"`
	Prompt   string `json:"prompt,omitempty"`
	Negative string `json:"negative,omitempty"`
}
type NewPromptRequest struct {
	Prompt   string  `json:"prompt,omitempty"`
	Negative *string `json:"negative,omitempty"`
}

type NewPromptResponse struct {
	Status opermanager.Status `json:"status"`
	Error  ErrorAttributes    `json:"error,omitempty"`
}

// ErrorResponse структура для исходящего ответа
type ErrorResponse struct {
	Error ErrorAttributes `json:"error,omitempty"`
}

// StartResponse структура для исходящего ответа
type StartResponse struct {
	ID     string             `json:"id,omitempty"`
	Status opermanager.Status `json:"status"`
	Error  ErrorAttributes    `json:"error,omitempty"`
}

// OperationStatusResponse структура для исходящего ответа
type OperationStatusResponse struct {
	ID     string             `json:"id"`
	Status opermanager.Status `json:"status"`
	Error  ErrorAttributes    `json:"error,omitempty"`
}

type PromptCardResponse struct {
	ID              int
	Text            string
	Negative        string
	PlaceholderKeys []string // только названия: ["name", "role", ...]
}

type ProviderInfo struct {
	Code string `json:"Code"`
	Name string `json:"Name"` // удобочитаемое название
}

type PromptsPageResponse struct {
	AlertMessages      []AlertMessage `json:"alerts"`
	Prompts            []PromptCardResponse
	GlobalPlaceholders []PlaceholderDetail
	Providers          []ProviderInfo
}

type PlaceholderDetail struct {
	Name   string   `json:"Name"`
	Values []string `json:"Values"`
}

type PromptDetail struct {
	ID           int                 `json:"ID"`
	Text         string              `json:"Text"`
	Negative     *string             `json:"Negative,omitempty"`
	Placeholders []PlaceholderDetail `json:"Placeholders"`
}

type PromptDetailRequest struct {
	ID           int                 `json:"ID"`
	Text         string              `json:"Text"`
	Negative     *string             `json:"Negative,omitempty"`
	Placeholders []PlaceholderDetail `json:"Placeholders"`
}

type GenerateByPromptRequest struct {
	PromptID int    `json:"PromptID"`
	Provider string `json:"Provider"`
}
