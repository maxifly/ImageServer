package ydart

import (
	"context"
	"fmt"
	"imgserver/internal/pkg/actioner"
	"imgserver/internal/pkg/imageprocessor"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/timerange"
	"imgserver/internal/pkg/ydapi"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"io"
	"log/slog"
	"net/http"
)

const (
	CoreBaseURL  string = "https://llm.api.cloud.yandex.net"
	ProviderCode        = "YandexArt"
	YANDEX_MODEL        = "aliceai-image-art-3.0"
	TMP_FILE            = "ydart_tmp.bin"
)

var _ opermanager.ImageProvider = (*YdArt)(nil)

type YdArtSleepTime struct {
	TimeRange *timerange.TimeRange `yaml:"time_range"`
}

type YdArtOptions struct {
	ImageGenerateThreshold int              `yaml:"image_generate_threshold"`
	SleepTimes             []YdArtSleepTime `yaml:"sleep_time"`
}

type YdArt struct {
	ydApi *ydapi.YdApi
	// ---
	logger          *slog.Logger
	options         *YdArtOptions
	imageParameters *opermanager.ImageParameters
	promptManager   *promptmanager.PromptManager
	actioner        *actioner.Actioner
	ipr             *imageprocessor.Ipr
	properties      *opermanager.ProviderProperties
	fileStorage     *FileStorage
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

func NewYdArt(imageParameters imageprocessor.ImageParameters, ydApi *ydapi.YdApi, promptManager *promptmanager.PromptManager, logger *slog.Logger, options *YdArtOptions) *YdArt {
	return &YdArt{
		ydApi:         ydApi,
		logger:        logger,
		options:       options,
		promptManager: promptManager,
		actioner:      actioner.NewActioner(options.ImageGenerateThreshold, time.Minute),
		ipr:           imageprocessor.NewIpr(imageParameters, logger),
		fileStorage:   NewFileStorage(),
		properties: &opermanager.ProviderProperties{
			IsCanWorkWithPrompt:  true,
			IsNeedSaveLocalFiles: true,
		},
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

	return ydArt.generateByPrompt(prompt, isDirectCall)
}

func (ydArt *YdArt) GenerateByParameters(parameters opermanager.GenerationParameters, isDirectCall bool) (string, error) {
	prompt := parameters.PromptText
	if parameters.NegativeText != nil && strings.Trim(*parameters.NegativeText, "") != "" {
		prompt = prompt + ". Игнорировать следующее: " + *parameters.NegativeText
	}
	return ydArt.generateByPrompt(prompt, isDirectCall)
}

func (ydArt *YdArt) generateByPrompt(prompt string, isDirectCall bool) (string, error) {

	if prompt == "" {
		return "", fmt.Errorf("prompt is empty")
	}

	ydArt.logger.Debug("generate with prompt", "prompt", prompt, "isDirect", isDirectCall)

	if !isDirectCall {
		ydArt.actioner.SetLastCallTime(time.Now())
	}
	err := ydArt.generateByOpenAi(prompt)
	if err != nil {
		resultError := fmt.Errorf("error generate image: %w", err)
		ydArt.logger.Error(resultError.Error())
		return "", err
	}
	timestamp := time.Now().UnixNano()
	operationId := fmt.Sprintf("ydrOapiGen_%d", timestamp)
	ydArt.logger.Debug("Generate by prompt completed", "operationId", operationId)
	return operationId, nil
}

// Deprecated: old async realisation
//func (ydArt *YdArt) generateByYdArtApi(prompt string, isDirectCall bool) (string, error) {
//
//	if prompt == "" {
//		return "", fmt.Errorf("prompt is empty")
//	}
//
//	ydArt.logger.Debug("generate with prompt", "prompt", prompt, "isDirect", isDirectCall)
//
//	generatePromptMessage := generatePrompt{Text: prompt,
//		Weight: 1}
//
//	ratio := aspectRatio{
//		HeightRatio: strconv.Itoa(ydArt.imageParameters.Height),
//		WidthRatio:  strconv.Itoa(ydArt.imageParameters.Weight),
//	}
//
//	generationOpt := generationOptions{
//		MimeType:    "image/jpeg",
//		AspectRatio: &ratio,
//	}
//
//	request := generateRequest{
//		ModelUri:          "art://" + ydArt.ydApi.GetFolderId() + "/yandex-art/latest",
//		Messages:          []*generatePrompt{&generatePromptMessage},
//		GenerationOptions: &generationOpt,
//	}
//
//	url := fmt.Sprintf("%s/foundationModels/v1/imageGenerationAsync", CoreBaseURL)
//	var response getImageResponse
//	err := ydArt.innerRequest("POST", url, http.StatusOK, request, &response)
//
//	if err != nil {
//		resultError := fmt.Errorf("error generate image: %w", err)
//		ydArt.logger.Error(resultError.Error())
//		return "", resultError
//	}
//
//	if response.Error != "" {
//		resultError := fmt.Errorf("YdArt return error: %v %v", response.ErrorCode, response.ErrorMessage)
//		ydArt.logger.Error(resultError.Error())
//		return "", resultError
//	}
//
//	if response.Id == "" {
//		resultError := fmt.Errorf("YdArt return empty operatioId")
//		ydArt.logger.Error(resultError.Error())
//		return "", resultError
//	}
//
//	ydArt.logger.Debug("YandexArt operation id", "id", response.Id)
//	return response.Id, nil
//}

func (ydArt *YdArt) generateByOpenAi(prompt string) error {

	model := fmt.Sprintf("art://%s/%s", ydArt.ydApi.GetFolderId(), YANDEX_MODEL)
	//TODO Обработать ошибки правильно
	//TODO Сохранить результат в промежуточный файл
	//TODO Отдать как-то файл в другом методе
	response, err := ydArt.ydApi.GetOapiClient().Images.Generate(context.Background(), openai.ImageGenerateParams{
		Model:  openai.ImageModel(model),
		Prompt: prompt,
		Size:   openai.ImageGenerateParamsSize1024x1536,
	})
	if err != nil {
		resultError := fmt.Errorf("create image error: %w", err)
		ydArt.logger.Error(resultError.Error())
		return err
	}

	//imageBytes, err := base64.StdEncoding.DecodeString(response.Data[0].B64JSON)
	imageBytes, err := ydArt.ipr.ConvertBase64ToJpg(response.Data[0].B64JSON)
	if err != nil {
		resultError := fmt.Errorf("decode error: %w", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}

	err = ydArt.fileStorage.SaveToFile(TMP_FILE, imageBytes)
	if err != nil {
		resultError := fmt.Errorf("save result to file error: %w", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}

	return nil
}

func (ydArt *YdArt) GetImageSlice(operationId string) (bool, []byte, error) {
	ydArt.logger.Debug("Get image request")

	imageData, err := ydArt.fileStorage.LoadFromFile(TMP_FILE)
	if err != nil {
		resultError := fmt.Errorf("error when read temporary image file: %w", err)
		ydArt.logger.Error(resultError.Error())
		return true, nil, resultError
	}

	return true, imageData, nil
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

func (ydArt *YdArt) Start() error {
	return nil
}

func (ydArt *YdArt) GetProperties() *opermanager.ProviderProperties {
	return ydArt.properties
}

func (ydArt *YdArt) getPrompt() (string, error) {
	prompt, err := ydArt.promptManager.GetRandomPromptValue()
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

func (ydArt *YdArt) logBody(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		resultError := fmt.Errorf("error when read error body: %w", err)
		ydArt.logger.Error(resultError.Error())
		return
	}
	ydArt.logger.Error("Get response: %s ", string(body))

}
