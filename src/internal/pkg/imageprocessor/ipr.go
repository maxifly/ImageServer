package imageprocessor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"github.com/disintegration/imaging"
	"golang.org/x/image/draw"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
	"os"
)

type ImageParameters struct {
	ImageWeight  int
	ImageHeight  int
	FitThreshold float64
}

type Ipr struct {
	imageParameters ImageParameters
	logger          *slog.Logger
}

func NewIpr(imageParameters ImageParameters, logger *slog.Logger) *Ipr {
	return &Ipr{logger: logger,
		imageParameters: imageParameters}
}

func (ipr *Ipr) ProcessImageFromFile(fileName string, fileNameOriginalSize string, sourceFile string, targetW, targetH int) error {
	imgBytes, err := os.ReadFile(sourceFile)
	if err != nil {
		return fmt.Errorf("error when read file %v", err)
	}

	//return ipr.ProcessImageFromSLice(fileName, fileNameOriginalSize, imgBytes, targetW, targetH)
	fit, original, err := ipr.ProcessImageFromSLice(imgBytes, targetW, targetH, len(fileNameOriginalSize) > 0)
	if err != nil {
		ipr.logger.Error("Error process image from slice", "err", err)
		return err
	}

	err = writeFile(fileName, fit)
	if err != nil {
		ipr.logger.Error("Error save fit file", "filePath", fileName, "err", err)
		return err
	}

	if original != nil {
		err = writeFile(fileNameOriginalSize, original)
		if err != nil {
			ipr.logger.Error("Error save original file", "filePath", fileNameOriginalSize, "err", err)
			return err
		}
	}

	return nil
}

func (ipr *Ipr) ProcessImageFromBase64(fileName string, fileNameOriginalSize string, imageBase64 string, targetW, targetH int) error {
	// Декодирование Base64
	imgBytes, err := base64.StdEncoding.DecodeString(imageBase64)
	if err != nil {
		return fmt.Errorf("error when decode Base64: %v", err)
	}

	fit, original, err := ipr.ProcessImageFromSLice(imgBytes, targetW, targetH, len(fileNameOriginalSize) > 0)
	if err != nil {
		ipr.logger.Error("Error process image from slice", "err", err)
		return err
	}

	err = writeFile(fileName, fit)
	if err != nil {
		ipr.logger.Error("Error save fit file", "filePath", fileName, "err", err)
		return err
	}

	if original != nil {
		err = writeFile(fileNameOriginalSize, original)
		if err != nil {
			ipr.logger.Error("Error save original file", "filePath", fileNameOriginalSize, "err", err)
			return err
		}
	}

	return nil
}

// ProcessImageFromSLice  Обрабатывает изображение из массива. Ответ: (fit, original, error)
func (ipr *Ipr) ProcessImageFromSLice(imgData []byte, targetW, targetH int, wantOriginal bool) ([]byte, []byte, error) {

	// Декодируем изображение из байтов
	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode image: %w", err)
	}

	origW := src.Bounds().Dx()
	origH := src.Bounds().Dy()

	// Если размер уже точный — конвертируем в JPEG и возвращаем
	if origW == targetW && origH == targetH {
		ipr.logger.Debug("Image have target size")
		encoded, err := encodeJPEG(src)
		if err != nil {
			return nil, nil, err
		}

		return encoded, encoded, nil

	}

	// Проверяем разницу в соотношении сторон
	origRatio := float64(origW) / float64(origH)
	targetRatio := float64(targetW) / float64(targetH)
	diff := math.Abs(origRatio-targetRatio) / math.Min(origRatio, targetRatio)

	var result image.Image

	if diff <= ipr.imageParameters.FitThreshold {
		// Малое отклонение → просто растягиваем
		ipr.logger.Debug("Resize image", "diff", diff, "threshold", ipr.imageParameters.FitThreshold)
		result = imaging.Resize(src, targetW, targetH, imaging.Lanczos)
	} else {
		// Большое отклонение → fit + pad
		ipr.logger.Debug("Fit and pad image", "diff", diff, "threshold", ipr.imageParameters.FitThreshold)
		fit := imaging.Fit(src, targetW, targetH, imaging.Lanczos)
		result = padImage(fit, targetW, targetH)
	}

	encodedProcessed, err := encodeJPEG(result)
	if err != nil {
		return nil, nil, err
	}

	var encodedOriginal []byte
	if wantOriginal {
		encodedOriginal, err = encodeJPEG(src)
		if err != nil {
			return nil, nil, err
		}
	}

	return encodedProcessed, encodedOriginal, nil

}

// padImage дополнение изображения до targetW × targetH чёрным фоном, по центру.
func padImage(src image.Image, targetW, targetH int) image.Image {
	// Создаём новое RGBA-изображение нужного размера
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	// Заливаем чёрным
	black := color.RGBA{A: 255}
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: black}, image.Point{}, draw.Src)

	// Центрируем исходное изображение
	srcBounds := src.Bounds()
	srcW, srcH := srcBounds.Dx(), srcBounds.Dy()
	x := (targetW - srcW) / 2
	y := (targetH - srcH) / 2

	// Копируем src в центр dst
	draw.Draw(dst, srcBounds.Add(image.Point{X: x, Y: y}), src, srcBounds.Min, draw.Src)

	return dst
}

func encodeJPEG(img image.Image) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 95})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeFile(filePath string, src []byte) error {
	// Просто пишем байты в файл — никакой дополнительной обработки не нужно!
	err := os.WriteFile(filePath, src, 0644)
	if err != nil {

		return err
	}
	return nil
}
