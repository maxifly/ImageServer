package opermanager

import (
	"fmt"
	"image"
	"image/jpeg"
	"imgserver/internal/pkg/actioner"
	"imgserver/internal/pkg/dirmanager"
	"imgserver/internal/pkg/imageprocessor"
	"imgserver/internal/pkg/metrics"
	"imgserver/internal/pkg/timerange"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
)

// Определяем тип Status как новый тип на основе int
type generatorType int

// Определяем константы для статусов с использованием iota
const (
	YandexArt  generatorType = iota // 0
	OldPicture                      // 1
)

const BLACK_FILE_NAME = "black.jpeg"
const TEMPORARY_IMAGE_DIR = "tmp_images"

type Status string

// Определяем константы для статусов
const (
	StatusUnknown Status = "unknown"
	StatusPending Status = "pending"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

const (
	METRIC_TEMPLATE_OPERATION_START  = "OPERATION_START_"
	METRIC_TEMPLATE_OPERATION_STATUS = "OPERATION_STATUS_"
	METRIC_TEMPLATE_IMAGE_GET        = "IMAGE_GET_"
)

type SleepTime struct {
	TimeRange      *timerange.TimeRange `yaml:"time_range"`
	BlackImageMode bool                 `yaml:"black_image_mode"`
}

type OperMngr struct {
	pendingOperations        *cache.Cache
	completeOperations       *cache.Cache
	logger                   *slog.Logger
	imageProviders           []*ImageProvider
	imageProvidersWithPrompt []*ImageProvider
	//ydArt              *ydart.YdArt
	dirManager      *dirmanager.DirManager
	dirManagerTemp  *dirmanager.DirManager
	idMutex         *IdMutex
	actioner        *actioner.Actioner
	sleepTimes      []*SleepTime
	imageParameters ImageParameters
	//TODO create
	ipr     *imageprocessor.Ipr
	metrics *metrics.AppMetrics
}
type OperStatus struct {
	Status Status
	Error  string
}

type Operation struct {
	Id         string
	Provider   *ImageProvider
	ExternalId string
	FileName   string
	Type       generatorType
	status     *OperStatus
}

func NewOperMngr(thresholdMinutes int,
	imageParameters imageprocessor.ImageParameters,
	sleepTimes []*SleepTime,
	dirManager *dirmanager.DirManager,
	metrics *metrics.AppMetrics,
	logger *slog.Logger) (*OperMngr, error) {
	pendingOperations := cache.New(1*time.Hour, 2*time.Hour)
	completeOperations := cache.New(1*time.Hour, 2*time.Hour)

	dirManagerTemp, err := dirmanager.NewDirManager(TEMPORARY_IMAGE_DIR, 5, 10, logger)

	if err != nil {
		return nil, err
	}

	operMng := OperMngr{
		pendingOperations:  pendingOperations,
		completeOperations: completeOperations,
		dirManager:         dirManager,
		dirManagerTemp:     dirManagerTemp,
		logger:             logger,
		idMutex:            NewIdMutex(),
		actioner:           actioner.NewActioner(thresholdMinutes, time.Minute),
		sleepTimes:         sleepTimes,
		imageParameters:    ImageParameters{Height: imageParameters.ImageHeight, Weight: imageParameters.ImageWeight},
		metrics:            metrics,
		ipr:                imageprocessor.NewIpr(imageParameters, logger),
	}
	return &operMng, nil
}

func (op *OperMngr) AddImageProvider(imageProvider *ImageProvider) {
	op.imageProviders = append(op.imageProviders, imageProvider)
	if (*imageProvider).GetProperties().IsCanWorkWithPrompt {
		op.imageProvidersWithPrompt = append(op.imageProvidersWithPrompt, imageProvider)
	}
}

func (op *OperMngr) Start() error {
	if len(op.imageProviders) == 0 {
		return fmt.Errorf("no image providers found")
	}
	err := op.createBlackJPEGSafe(op.imageParameters.Weight, op.imageParameters.Height, BLACK_FILE_NAME)
	if err != nil {
		return err
	}

	for _, st := range op.sleepTimes {
		_, err = st.TimeRange.IsWithinRangeInclusive(time.Now())
		if err != nil {
			return err
		}
	}

	err = op.dirManagerTemp.Start()
	if err != nil {
		op.logger.Error("Error start temporary directory manager", "error", err)
		return fmt.Errorf("error start temporary directory manager: %v", err)
	}

	for _, provider := range op.imageProviders {
		op.logger.Info("Start image provider", "provider", (*provider).GetImageProviderForImageServerName())
		err := (*provider).Start()
		if err != nil {
			op.logger.Error("Error start image provider", "provider", (*provider).GetImageProviderForImageServerName(), "error", err)
			return fmt.Errorf("error start provider: %v", err)
		}
	}

	return nil
}

func (op *OperMngr) StartOperation(optype string, prompt string) (string, error) {
	//op.metrics.TotalRequests.Inc(1)
	if optype == "ydart" {
		op.logger.Info("Start direct provider operation")
		provider := op.getImageProvider(len(prompt) > 0)
		return op.startProviderOperation(provider, prompt, true)
	} else if optype == "old" {
		return op.startOldPictureOperation()
	}
	return op.startAutoOperation()

}

func (op *OperMngr) getImageProvider(withPrompt bool) *ImageProvider {
	if withPrompt {
		if len(op.imageProvidersWithPrompt) == 1 {
			op.logger.Debug("Get provider", "provider", (*op.imageProvidersWithPrompt[0]).GetImageProviderForImageServerName())
			return op.imageProvidersWithPrompt[0]
		}

		idx := rand.Intn(len(op.imageProvidersWithPrompt))
		op.logger.Debug("Get provider", "provider", (*op.imageProvidersWithPrompt[idx]).GetImageProviderForImageServerName())

		return op.imageProvidersWithPrompt[idx]
	} else {

		if len(op.imageProviders) == 1 {
			op.logger.Debug("Get provider", "provider", (*op.imageProviders[0]).GetImageProviderForImageServerName())
			return op.imageProviders[0]
		}

		idx := rand.Intn(len(op.imageProviders))
		op.logger.Debug("Get provider", "provider", (*op.imageProviders[idx]).GetImageProviderForImageServerName())

		return op.imageProviders[idx]
	}

}

func (op *OperMngr) chooseImageProvider() *ImageProvider {
	// Перебираем провайдеров которые могут принять задание
	var readyProviders []int
	for idx, pr := range op.imageProviders {
		if (*pr).IsReadyForRequest() {
			readyProviders = append(readyProviders, idx)
		}
	}

	if len(readyProviders) == 0 {
		return nil
	} else if len(readyProviders) == 1 {
		op.logger.Debug("Choose provider", "provider", (*op.imageProviders[readyProviders[0]]).GetImageProviderForImageServerName())
		return op.imageProviders[readyProviders[0]]
	}

	idx := rand.Intn(len(readyProviders))
	op.logger.Debug("Choose provider", "provider", (*op.imageProviders[readyProviders[idx]]).GetImageProviderForImageServerName())

	return op.imageProviders[idx]
}

func (op *OperMngr) startAutoOperation() (string, error) {
	op.logger.Info("Start auto operation")
	now := time.Now()

	st := op.checkSleepTime(now)

	// Сейчас период сна. Посмотрим что надо сделать.
	if st != nil {
		if st.BlackImageMode {
			return op.startBlackPictureOperation()
		} else {
			return op.startOldPictureOperation()
		}
	}

	if op.actioner.ThresholdOut(now) {
		op.logger.Debug("Threshold")
		// Внешний провайдер давно не вызывался
		provider := op.chooseImageProvider()

		if provider == nil {
			op.logger.Debug("Ready provider is nil")
			// Вызываем менеджер старых изображений
			return op.startOldPictureOperation()
		}
		operation, err := op.startProviderOperation(provider, "", false)
		if err != nil {
			return "", err
		}
		// Обновляем время последнего вызова
		op.actioner.SetLastCallTime(now)
		return operation, nil
	} else {
		// Вызываем менеджер старых изображений
		return op.startOldPictureOperation()
	}
}

func (op *OperMngr) startBlackPictureOperation() (string, error) {
	op.logger.Info("Start black picture operation")
	return op.startGetOldPictureFromLocalStorageOperation(true)
}
func (op *OperMngr) startOldPictureOperation() (string, error) {
	op.logger.Info("Start old picture operation")
	return op.startGetOldPictureFromLocalStorageOperation(false)
}

func (op *OperMngr) startGetOldPictureFromLocalStorageOperation(getBlackPicture bool) (string, error) {
	id := op.generateId()
	var file string
	if getBlackPicture {
		file = BLACK_FILE_NAME
	} else {
		originalFile := op.dirManager.GetRandomFile()
		imgBytes, err := os.ReadFile(originalFile)

		if err != nil {
			return id, fmt.Errorf("error when read file %v", err)
		}

		file, err = op.saveFiles(id, imgBytes, false)
		if err != nil {
			return id, err
		}

	}

	op.logger.Info("Start old picture operation", "file", file)
	operation := Operation{
		Id:         id,
		ExternalId: "dirManagerOperation",
		Type:       OldPicture,
		FileName:   file,
		status: &OperStatus{
			Status: StatusDone,
			Error:  "",
		},
	}
	op.completeOperations.SetDefault(operation.Id, &operation)
	op.logger.Info("Start old picture operation", "operationId", operation.Id, "file", operation.FileName)
	return operation.Id, nil

}

func (op *OperMngr) startProviderOperation(provider *ImageProvider, prompt string, isDirectCall bool) (string, error) {
	op.logger.Info("Start provider operation", "isDirectCall", isDirectCall)

	providerMetric := op.metrics.GetRequestTypeMetricsSafe(METRIC_TEMPLATE_OPERATION_START + (*provider).GetImageProviderCode())

	var externalId string
	var err error

	if prompt != "" {
		op.logger.Debug("Start provider operation with prompt")
		externalId, err = (*provider).GenerateWithPrompt(strings.Trim(prompt, " "), isDirectCall)
	} else {
		externalId, err = (*provider).Generate(isDirectCall)
	}

	if err != nil {
		resultError := fmt.Errorf("error provider generate %v", err)
		op.logger.Error("Can not start operation", "error", resultError)
		providerMetric.IncrementErrorRequest()

		return "", resultError
	}

	providerMetric.IncrementSuccessRequest()
	op.metrics.IncrementDaily(METRIC_TEMPLATE_OPERATION_START + (*provider).GetImageProviderCode())

	operation := Operation{
		Id:         op.generateId(),
		Provider:   provider,
		ExternalId: externalId,
		Type:       YandexArt,
		status: &OperStatus{
			Status: StatusPending,
			Error:  "",
		},
	}
	op.pendingOperations.SetDefault(operation.Id, &operation)
	return operation.Id, nil
}

func (op *OperMngr) GetOperationStatus(id string) (*OperStatus, error) {
	lock := op.idMutex.GetLock(id)
	defer op.idMutex.ReleaseLock(id)
	lock.Lock()
	defer lock.Unlock()
	operation, ok := op.pendingOperations.Get(id)

	if !ok {
		operation, ok := op.completeOperations.Get(id)
		if !ok {
			resultError := fmt.Errorf("operation not found %v", id)
			op.logger.Error("Operation not found", "error", resultError)
			return nil, resultError
		}

		return operation.(*Operation).status, nil
	}
	provider := operation.(*Operation).Provider

	isNeedSaveLocalFiles := (*provider).GetProperties().IsNeedSaveLocalFiles

	ydOperationResult, imageData, err := (*provider).GetImageSlice(operation.(*Operation).ExternalId)
	if err != nil {
		return nil, err
	}

	if ydOperationResult {
		op.logger.Debug("Operation completed", "id", operation.(*Operation).Id)
		operStatus := &OperStatus{Status: StatusDone, Error: ""}

		fileName, err := op.saveFiles(id, imageData, isNeedSaveLocalFiles)
		if err != nil {
			operStatus = &OperStatus{Status: StatusError, Error: err.Error()}
		}

		op.logger.Debug("Operation completed", "id", operation.(*Operation).Id, "fileName", fileName)

		completeOperation := operation.(*Operation)
		completeOperation.status = operStatus
		completeOperation.FileName = fileName
		op.completeOperations.SetDefault(id, completeOperation)
		op.pendingOperations.Delete(id)

	}

	return &OperStatus{Status: StatusPending}, nil
}

func (op *OperMngr) saveFiles(id string, imageData []byte, isNeedSaveLocalFiles bool) (string, error) {
	if isNeedSaveLocalFiles {
		fileNameOrig := op.generateFileName(id)
		err := writeFile(fileNameOrig, imageData)
		if err != nil {
			op.logger.Error("Can not save local original file", "error", err)
		}
		op.dirManager.AddFile(fileNameOrig)
	}

	// Сконвертируем изображение к целевому размеру

	fileName := op.generateTemporaryFileName(id)

	fit, _, err := op.ipr.ProcessImageFromSLice(imageData, op.imageParameters.Weight, op.imageParameters.Height, false)
	if err != nil {
		return "", err
	}

	err = writeFile(fileName, fit)
	if err != nil {
		return "", err
	}
	op.dirManagerTemp.AddFile(fileName)

	return fileName, nil
}

func (op *OperMngr) GetFileName(id string) (string, error) {

	operation, ok := op.completeOperations.Get(id)
	if !ok {
		resultError := fmt.Errorf("operation not complete %v", id)
		op.logger.Info("Operation not complete")
		return "", resultError
	}

	return operation.(*Operation).FileName, nil

}

func (op *OperMngr) CheckPendingOperations() {
	op.logger.Debug("Check pending operations")

	count := op.pendingOperations.ItemCount()

	ids := make([]string, 0, count+10)
	for _, k := range op.pendingOperations.Items() {
		op.logger.Debug("Pending operation", "operationId", k.Object.(*Operation).Id)
		ids = append(ids, k.Object.(*Operation).Id)
	}

	for _, id := range ids {
		_, err := op.GetOperationStatus(id)
		if err != nil {
			op.logger.Error("error when check pending operation", "error", err, "id", id)
		}
	}
}

func (op *OperMngr) checkSleepTime(now time.Time) *SleepTime {

	for _, st := range op.sleepTimes {
		inclusive, err := st.TimeRange.IsWithinRangeInclusive(now)
		if err != nil {
			op.logger.Error("Get time range error", "error", err)
		}
		if inclusive {
			return st
		}
	}
	return nil
}

func (op *OperMngr) createBlackJPEGSafe(width, height int, filename string) error {
	// Проверяем существование файла
	if _, err := os.Stat(filename); err == nil {
		// Файл существует - удаляем его
		if err := os.Remove(filename); err != nil {
			op.logger.Error("Remove old image file failed", "error", err, "filename", filename)
			return fmt.Errorf("can not delete file %s: %w", filename, err)
		}
		op.logger.Debug("Remove old image file success", "filename", filename)
	} else if !os.IsNotExist(err) {
		// Другая ошибка при проверке существования
		op.logger.Error("Check image file failed", "error", err, "filename", filename)
		return fmt.Errorf("check image file error: %w", err)
	}

	// Создаем новое изображение
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Заполняем черным цветом
	op.fillBlack24Bit(img)

	file, err := os.Create(filename)
	if err != nil {
		op.logger.Error("Create image file failed", "error", err, "filename", filename)
		return fmt.Errorf("create image file error: %w", err)
	}
	defer file.Close()

	// Сохраняем как JPEG

	jpegOptions := &jpeg.Options{
		Quality: 100, // Качество сжатия
		// Color model и bit depth будут автоматически определены из RGBA
	}

	if err := jpeg.Encode(file, img, jpegOptions); err != nil {
		op.logger.Error("Create JPEG image file failed", "error", err, "filename", filename)
		return fmt.Errorf("create JPEG image file error: %w", err)
	}

	op.logger.Debug("Create image file success", "filename", filename)
	return nil
}

func (op *OperMngr) fillBlack24Bit(rgba *image.RGBA) {
	bounds := rgba.Bounds()
	pixels := bounds.Max.X * bounds.Max.Y

	op.logger.Debug(fmt.Sprintf("Fill %d pixels black color...", pixels))

	// Устанавливаем все пиксели в черный цвет

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		rowStart := y * rgba.Stride
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixelPos := rowStart + x*4

			rgba.Pix[pixelPos] = 0     // R = 0 (черный)
			rgba.Pix[pixelPos+1] = 0   // G = 0 (черный)
			rgba.Pix[pixelPos+2] = 0   // B = 0 (черный)
			rgba.Pix[pixelPos+3] = 255 // A = 255 (непрозрачный)
		}
	}

	op.logger.Debug("Fill pixels completed")
}

func (op *OperMngr) generateId() string {
	unixSeconds := time.Now().Unix()
	return "i" + strconv.Itoa(int(unixSeconds))
}

func (op *OperMngr) generateFileName(id string) string {
	unixSeconds := time.Now().Unix()
	orig := "f" + strconv.Itoa(int(unixSeconds)) + "-orig.jpeg"

	return filepath.Join(op.dirManager.GetDirectoryPath(), orig)
}

func (op *OperMngr) generateTemporaryFileName(id string) string {
	unixSeconds := time.Now().Unix()
	small := "f" + strconv.Itoa(int(unixSeconds)) + ".jpeg"

	return filepath.Join(op.dirManagerTemp.GetDirectoryPath(), small)
}

func writeFile(filePath string, src []byte) error {
	// Просто пишем байты в файл — никакой дополнительной обработки не нужно!
	err := os.WriteFile(filePath, src, 0644)
	if err != nil {

		return err
	}
	return nil
}
