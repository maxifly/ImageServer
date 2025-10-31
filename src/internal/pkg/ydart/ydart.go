package ydart

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"imgserver/internal/pkg/actioner"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/timerange"
	"strconv"
	"strings"
	"time"

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
	ProviderCode             = "YandexArt"
)

var _ opermanager.ImageProvider = (*YdArt)(nil)

type YdArtSecretOption struct {
	FolderId string `json:"folder_id"`
	ApiKey   string `json:"api_key"`
}

type YdArtSleepTime struct {
	TimeRange *timerange.TimeRange `yaml:"time_range"`
}

type YdArtOptions struct {
	ImageGenerateThreshold int              `yaml:"image_generate_threshold"`
	SleepTimes             []YdArtSleepTime `yaml:"sleep_time"`
}

type YdArt struct {
	httpClient      *http.Client
	logger          *slog.Logger
	soptions        *YdArtSecretOption
	options         *YdArtOptions
	imageParameters *opermanager.ImageParameters
	promptManager   *promptmanager.PromptManager
	actioner        *actioner.Actioner
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

func NewYdArt(promptManager *promptmanager.PromptManager, logger *slog.Logger, options *YdArtOptions) *YdArt {
	soptions, err := readSecretOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Yandex art options: %s, %v", FILE_PATH_OPTIONS, err))
	}
	//logger.Debug("Options ", "options", options)
	return &YdArt{
		httpClient:    http.DefaultClient,
		logger:        logger,
		soptions:      &soptions,
		options:       options,
		promptManager: promptManager,
		actioner:      actioner.NewActioner(options.ImageGenerateThreshold, time.Minute),
	}
}

func (ydArt *YdArt) SetImageParameters(parameters *opermanager.ImageParameters) error {
	ydArt.imageParameters = parameters
	return nil
}
func (ydArt *YdArt) GetImageProviderForImageServerName() string {
	return "YandexArt"
}
func (ydArt *YdArt) GetImageProviderCode() string {
	return ProviderCode
}

func (ydArt *YdArt) Generate(isDirectCall bool) (string, error) {
	prompt, err := ydArt.getPrompt()
	if err != nil {
		return "", err
	}
	return ydArt.GenerateWithPrompt(prompt, isDirectCall)
}

func (ydArt *YdArt) GenerateWithPrompt(prompt string, isDirectCall bool) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is empty")
	}

	ydArt.logger.Debug("generate with prompt", "prompt", prompt, "isDirect", isDirectCall)

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
		ModelUri:          "art://" + ydArt.soptions.FolderId + "/yandex-art/latest",
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

	if !isDirectCall {
		ydArt.actioner.SetLastCallTime(time.Now())
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

func (ydArt *YdArt) IsReadyForRequest() bool {
	if !ydArt.actioner.ThresholdOut(time.Now()) {
		// Провайдер вызывался недавно. Он не готов к новому вызову.
		return false
	}

	// Проверяем не наступило ли время сна
	if len(ydArt.options.SleepTimes) > 0 {
		now := time.Now()
		for _, st := range ydArt.options.SleepTimes {
			//ydArt.logger.Debug("Get time range", "time", st.TimeRange)
			inclusive, err := st.TimeRange.IsWithinRangeInclusive(now)
			if err != nil {
				ydArt.logger.Error("Get time range error", "error", err)
			}
			if inclusive {
				return false
			}

		}
	}

	return true
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
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", ydArt.soptions.ApiKey))
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

func readSecretOptions() (YdArtSecretOption, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data YdArtSecretOption
	err := json.Unmarshal(plan, &data)
	return data, err
}
