package ydart

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"imgserver/internal/pkg/promptmanager"
	"strconv"
	"strings"

	//"image/draw"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
)

const (
	FILE_PATH_OPTIONS        = "/data/ydart-options.json"
	CoreBaseURL       string = "https://llm.api.cloud.yandex.net"
)

type ImageParameters struct {
	Height int
	Weight int
}
type YdArtOption struct {
	FolderId string `json:"folder_id"`
	ApiKey   string `json:"api_key"`
}
type YdArt struct {
	httpClient      *http.Client
	logger          *slog.Logger
	options         *YdArtOption
	imageParameters *ImageParameters
	promptManager   *promptmanager.PromptManager
}

type getImageResponse struct {
	Id           string        `json:"id"`
	Done         bool          `json:"done"`
	Error        string        `json:"error"`
	ErrorCode    string        `json:"code"`
	ErrorMessage string        `json:"message"`
	ErrorDetails []string      `json:"details"`
	Response     imageResponse `json:"response"`
}

type generateRequest struct {
	ModelUri          string             `json:"model_uri"`
	Messages          []*generatePrompt  `json:"messages"`
	GenerationOptions *generationOptions `json:"generation_options"`
}

type generationOptions struct {
	MimeType    string       `json:"mime_type"`
	AspectRatio *aspectRatio `json:"aspectRatio"`
}

type generatePrompt struct {
	Text   string `json:"text"`
	Weight int    `json:"weight"`
}

type aspectRatio struct {
	WidthRatio  string `json:"widthRatio"`
	HeightRatio string `json:"heightRatio"`
}

type imageResponse struct {
	Image string `json:"image"`
}

func NewYdArt(imageParameters *ImageParameters, promptManager *promptmanager.PromptManager, logger *slog.Logger) *YdArt {
	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Yandex art options: %s, %v", FILE_PATH_OPTIONS, err))
	}
	//logger.Debug("Options ", "options", options)
	return &YdArt{
		httpClient:      http.DefaultClient,
		logger:          logger,
		options:         &options,
		imageParameters: imageParameters,
		promptManager:   promptManager,
	}
}

func (ydArt *YdArt) GetImageProviderForImageServerName() string {
	return "YandexArt"
}

func (ydArt *YdArt) Generate() (string, error) {
	prompt, err := ydArt.getPrompt()
	if err != nil {
		return "", err
	}
	return ydArt.GenerateWithPrompt(prompt)
}

func (ydArt *YdArt) GenerateWithPrompt(prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is empty")
	}

	generatePromptMessage := generatePrompt{Text: prompt,
		Weight: 1}

	ratio := aspectRatio{
		HeightRatio: strconv.Itoa(ydArt.imageParameters.Height),
		WidthRatio:  strconv.Itoa(ydArt.imageParameters.Weight),
	}

	generationOpt := generationOptions{
		MimeType:    "image/jpeg",
		AspectRatio: &ratio,
	}

	request := generateRequest{
		ModelUri:          "art://" + ydArt.options.FolderId + "/yandex-art/latest",
		Messages:          []*generatePrompt{&generatePromptMessage},
		GenerationOptions: &generationOpt,
	}

	url := fmt.Sprintf("%s/foundationModels/v1/imageGenerationAsync", CoreBaseURL)
	var response getImageResponse
	err := ydArt.innerRequest("POST", url, http.StatusOK, request, &response)

	if err != nil {
		resultError := fmt.Errorf("error generate image: %v", err)
		ydArt.logger.Error(resultError.Error())
		return "", resultError
	}

	if response.Error != "" {
		resultError := fmt.Errorf("YdArt return error: %v %v", response.ErrorCode, response.ErrorMessage)
		ydArt.logger.Error(resultError.Error())
		return "", resultError
	}

	if response.Id == "" {
		resultError := fmt.Errorf("YdArt return empty operatioId")
		ydArt.logger.Error(resultError.Error())
		return "", resultError
	}

	ydArt.logger.Debug("YandexArt operation id", "id", response.Id)
	return response.Id, nil
}

func (ydArt *YdArt) getPrompt() (string, error) {
	prompt, err := ydArt.promptManager.GetRandomPrompt()
	if err != nil {
		ydArt.logger.Error("Error when get prompt", "error", err.Error())
		ydArt.logger.Debug("Return default prompt", "prompt", "test")
		return "test", err
	}

	result := prompt.Prompt
	if prompt.Negative != nil && strings.Trim(*prompt.Negative, "") != "" {
		result = result + ". Игнорировать следующее: " + *prompt.Negative
	}

	ydArt.logger.Debug("result prompt", "prompt", result)
	return result, nil
}

func (ydArt *YdArt) GetImage(operationId string, filename string) (bool, error) {
	ydArt.logger.Debug("Get image request")
	url := fmt.Sprintf("%s/operations/%s", CoreBaseURL, operationId)
	var response getImageResponse
	err := ydArt.innerRequest("GET", url, http.StatusOK, nil, &response)
	if err != nil {
		resultError := fmt.Errorf("error when get image: %v", err)
		return false, resultError
	}

	if response.Done {
		if response.Error != "" {
			resultError := fmt.Errorf("error from YandexArt: %s", response.Error)
			ydArt.logger.Error("YandexArt error", "errorCode", response.ErrorCode, "error", response.Error, "detail", response.ErrorDetails)
			return true, resultError
		}
		if response.Response.Image != "" {
			err := processImage(filename, response.Response.Image, ydArt.imageParameters.Weight, ydArt.imageParameters.Height)
			if err != nil {
				resultError := fmt.Errorf("error image processing: %v", err)
				ydArt.logger.Error(resultError.Error())
				return false, resultError
			}
			return true, nil
		} else {
			resultError := fmt.Errorf("field image is empty")
			return false, resultError
		}
	}
	// Задача не завершена
	return false, nil
}

func (ydArt *YdArt) innerRequest(method string, url string, expectedStatus int, requestBody interface{}, result interface{}) error {
	ydArt.logger.Debug("Execute get request", "url", url)

	var req *http.Request
	var err error

	if requestBody == nil {
		// Создаем новый HTTP-запрос
		req, err = http.NewRequest(method, url, nil)
	} else {
		// Преобразуем структуру в JSON
		jsonData, err1 := json.Marshal(requestBody)
		if err1 != nil {
			resultError := fmt.Errorf("error when data marshalling: %v", err1)
			ydArt.logger.Error("error when data marshaling", resultError)
			return resultError
		}

		req, err = http.NewRequest(method, url, bytes.NewBuffer(jsonData))
	}
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", ydArt.options.ApiKey))
	req.Header.Set("Accept", "application/json") // Ожидание ответа в формате JSON

	// ydArt.logger.Debug("request", "request", req)

	// Выполняем запрос
	resp, err := ydArt.httpClient.Do(req)
	if err != nil {
		resultError := fmt.Errorf("error when execute request: %v", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}

	defer resp.Body.Close()

	// Проверяем статус код
	if resp.StatusCode != expectedStatus {
		resultError := fmt.Errorf("unexpected status code: %s", resp.Status)
		ydArt.logger.Error(resultError.Error())
		ydArt.logBody(resp)
		return resultError
	}

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read body: %v", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}

	// Декодируем JSON-ответ
	if err := json.Unmarshal(body, result); err != nil {
		resultError := fmt.Errorf("error when parse body: %v", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}
	ydArt.logger.Debug("Get result ")
	return nil
}

func (ydArt *YdArt) logBody(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read error body: %v", err)
		ydArt.logger.Error(resultError.Error())
		return
	}
	ydArt.logger.Error("Get response: %s ", string(body))

}

func processImage(fileName string, imageBase64 string, width, height int) error {
	// Декодирование Base64
	imgBytes, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return fmt.Errorf("ошибка при декодировании Base64: %v", err)
	}

	// Декодирование изображения
	img, err := jpeg.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return fmt.Errorf("ошибка при декодировании изображения: %v", err)
	}

	// Создание нового изображения с указанными размерами
	newImg := image.NewRGBA(image.Rect(0, 0, width, height))

	// Масштабирование изображения
	draw.CatmullRom.Scale(newImg, newImg.Bounds(), img, img.Bounds(), draw.Over, nil)

	// Запись измененного изображения в файл
	outFile, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("ошибка при создании файла: %v", err)
	}
	defer outFile.Close()

	err = jpeg.Encode(outFile, newImg, nil)
	if err != nil {
		return fmt.Errorf("ошибка при записи изображения в файл: %v", err)
	}

	return nil
}

func readOptions() (YdArtOption, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data YdArtOption
	err := json.Unmarshal(plan, &data)
	return data, err
}
