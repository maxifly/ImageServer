package ydart

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/image/draw"
	"image"
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
}

type getImageResponse struct {
	Id string `json:"id"`
	//Description string `json:"description"`
	//CreatedAt   string `json:"createdAt"`
	//CreatedBy   string `json:"createdBy"`
	//ModifiedAt  string `json:"modifiedAt"`
	Done bool `json:"done"`
	//Metadata    string `json:"metadata"`
	Error        string        `json:"error"`
	ErrorCode    string        `json:"code"`
	ErrorMessage string        `json:"message"`
	ErrorDetails []string      `json:"details"`
	Response     imageResponse `json:"response"`
}

type imageResponse struct {
	Image string `json:"image"`
}

func NewYdArt(imageParameters *ImageParameters, logger *slog.Logger) *YdArt {
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
	}
}

func (ydArt *YdArt) GetImage(operationId string, filename string) (bool, error) {
	ydArt.logger.Debug("Get image request")
	url := fmt.Sprintf("%s/operations/%s", CoreBaseURL, operationId)
	var response getImageResponse
	err := ydArt.innerRequest("GET", url, http.StatusOK, &response)
	if err != nil {
		resultError := fmt.Errorf("error when get image: %v", err)
		return false, resultError
	}

	if response.Done {
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

	//TODO Надо как-то обработать возврат ошибки
	return false, nil
}

func (ydArt *YdArt) innerRequest(method string, url string, expectedStatus int, result interface{}) error {
	ydArt.logger.Debug("Execute get request %s", url)

	// Создаем новый HTTP-запрос
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		resultError := fmt.Errorf("error when create request: %v", err)
		ydArt.logger.Error(resultError.Error())
		return resultError
	}

	// Устанавливаем заголовок авторизации
	req.Header.Set("Authorization", fmt.Sprintf("Api-Key %s", ydArt.options.ApiKey))
	req.Header.Set("Accept", "application/json") // Ожидание ответа в формате JSON

	ydArt.logger.Debug("request %v", req)

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
