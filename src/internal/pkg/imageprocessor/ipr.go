package imageprocessor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"image/jpeg"
	"log/slog"
	"os"
)

type Ipr struct {
	logger *slog.Logger
}

func NewIpr(logger *slog.Logger) *Ipr {
	return &Ipr{logger: logger}
}

func (ipr *Ipr) ProcessImageFromFile(fileName string, fileNameOriginalSize string, sourceFile string, width, height int) error {
	// Декодирование Base64
	imgBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("error when read file %v", err)
	}

	return ipr.ProcessImageFromSLice(fileName, fileNameOriginalSize, imgBytes, width, height)
}

func (ipr *Ipr) ProcessImageFromBase64(fileName string, fileNameOriginalSize string, imageBase64 string, width, height int) error {
	// Декодирование Base64
	imgBytes, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return fmt.Errorf("error when decode Base64: %v", err)
	}

	return ipr.ProcessImageFromSLice(fileName, fileNameOriginalSize, imgBytes, width, height)
}

func (ipr *Ipr) ProcessImageFromSLice(fileName string, fileNameOriginalSize string, imgBytes []byte, width, height int) error {

	// Сохраняем оригинал
	if err := saveOriginalImage(fileNameOriginalSize, imgBytes); err != nil {
		ipr.logger.Error("Error when save original image", "error", err, "fileName", fileNameOriginalSize)
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

func saveOriginalImage(originalFileName string, imgBytes []byte) error {
	if len(originalFileName) > 0 {
		return os.WriteFile(originalFileName, imgBytes, 0644)
	}
	return nil
}
