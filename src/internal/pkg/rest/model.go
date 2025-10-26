package rest

import "imgserver/internal/pkg/opermanager"

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
type StatusResponse struct {
	TotalRequests            int64   `json:"total_requests"`
	TotalRequestsError       int64   `json:"total_requests_errors"`
	TotalRequestsSuccessRate float64 `json:"total_requests_success_rate"`
	TotalRequestsErrorRate   float64 `json:"total_requests_errors_rate"`

	ImagesSentTotal       int64   `json:"images_sent_total"`
	ImagesSentError       int64   `json:"images_sent_error"`
	ImagesSentSuccessRate float64 `json:"images_sent_success_rate"`
	ImagesSentErrorRate   float64 `json:"images_sent_error_rate"`

	YandexTotal       int64   `json:"yandex_total"`
	YandexError       int64   `json:"yandex_error"`
	YandexSuccessRate float64 `json:"yandex_success_rate"`
	YandexErrorRate   float64 `json:"yandex_error_rate"`
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
