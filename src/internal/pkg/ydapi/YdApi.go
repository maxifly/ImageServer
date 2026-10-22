package ydapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"io"
	"log/slog"
	"net/http"
	"os"
)

type RestMethod string

const (
	FILE_PATH_OPTIONS            = "/data/ydart-options.json"
	POST              RestMethod = "POST"
	GET               RestMethod = "GET"
)

type YdArtSecretOption struct {
	FolderId string `json:"folder_id"`
	ApiKey   string `json:"api_key"`
}
type YdApi struct {
	oapiClient openai.Client
	httpClient *http.Client
	logger     *slog.Logger
	soptions   *YdArtSecretOption
}

func NewYdApi(logger *slog.Logger) (*YdApi, error) {
	soptions, err := readSecretOptions()
	if err != nil {
		return nil, fmt.Errorf("can not read Yandex art options: %s, %v", FILE_PATH_OPTIONS, err)
	}
	//logger.Debug("Options ", "options", options)
	oapiClient := openai.NewClient(
		option.WithAPIKey(soptions.ApiKey),
		option.WithBaseURL("https://ai.api.cloud.yandex.net/v1"),
		option.WithProject(soptions.FolderId),
	)

	return &YdApi{
		oapiClient: oapiClient,
		httpClient: http.DefaultClient,
		logger:     logger,
		soptions:   &soptions,
	}, nil
}
func (ydApi *YdApi) GetFolderId() string {
	return ydApi.soptions.FolderId
}
func (ydApi *YdApi) GetOapiClient() *openai.Client {
	return &ydApi.oapiClient
}
func (ydApi *YdApi) Request(method RestMethod, url string, expectedStatus int, requestBody interface{}, result interface{}) error {
	ydApi.logger.Debug("Execute request", "method", method, "url", url)

	var req *http.Request
	var err error

	if requestBody == nil {
		// Создаем новый HTTP-запрос
		req, err = http.NewRequest(string(method), url, nil)
	} else {
		// Преобразуем структуру в JSON
		jsonData, err1 := json.Marshal(requestBody)
		if err1 != nil {
			resultError := fmt.Errorf("error when data marshalling: %w", err1)
			ydApi.logger.Error("error when data marshaling", resultError)
			return resultError
		}

		req, err = http.NewRequest(string(method), url, bytes.NewBuffer(jsonData))
	}
	if err != nil {
		resultError := fmt.Errorf("error when create request: %w", err)
		ydApi.logger.Error(resultError.Error())
		return resultError
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", ydApi.soptions.ApiKey))
	req.Header.Set("Accept", "application/json")              // Ожидание ответа в формате JSON
	req.Header.Set("OpenAI-Project", ydApi.soptions.FolderId) // Id проекта
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// ydApi.logger.Debug("request", "request", req)

	// Выполняем запрос
	resp, err := ydApi.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %w", err)
		ydApi.logger.Error(resultError.Error())
		return resultError
	}

	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode != expectedStatus {
		resultError := fmt.Errorf("unexpected status code: %s", resp.Status)
		ydApi.logger.Error(resultError.Error())
		ydApi.logBody(resp)
		return resultError
	}

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read body: %w", err)
		ydApi.logger.Error(resultError.Error())
		return resultError
	}

	// Декодируем JSON-ответ
	if err := json.Unmarshal(body, result); err != nil {
		resultError := fmt.Errorf("error when parse body: %w", err)
		ydApi.logger.Error(resultError.Error())
		return resultError
	}
	ydApi.logger.Debug("Get result ")
	return nil
}

func readSecretOptions() (YdArtSecretOption, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data YdArtSecretOption
	err := json.Unmarshal(plan, &data)
	return data, err
}

func (ydApi *YdApi) logBody(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read error body: %w", err)
		ydApi.logger.Error(resultError.Error())
		return
	}
	ydApi.logger.Error("Get response: %s ", string(body))

}
