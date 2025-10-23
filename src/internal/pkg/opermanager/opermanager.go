package opermanager

import (
	"fmt"
	"imgserver/internal/pkg/actioner"
	"imgserver/internal/pkg/dirmanager"
	"log/slog"
	"math/rand"
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

type Status string

// Определяем константы для статусов
const (
	StatusUnknown Status = "unknown"
	StatusPending Status = "pending"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

type OperMngr struct {
	directoryPath      string
	pendingOperations  *cache.Cache
	completeOperations *cache.Cache
	logger             *slog.Logger
	imageProviders     []*ImageProvider
	//ydArt              *ydart.YdArt
	dirManager *dirmanager.DirManager
	idMutex    *IdMutex
	actioner   *actioner.Actioner
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

func NewOperMngr(directoryPath string, thresholdMinutes int,
	dirManager *dirmanager.DirManager,
	logger *slog.Logger) *OperMngr {

	pendingOperations := cache.New(1*time.Hour, 2*time.Hour)
	completeOperations := cache.New(1*time.Hour, 2*time.Hour)

	operMng := OperMngr{
		directoryPath:      directoryPath,
		pendingOperations:  pendingOperations,
		completeOperations: completeOperations,
		dirManager:         dirManager,
		logger:             logger,
		idMutex:            NewIdMutex(),
		actioner:           actioner.NewActioner(thresholdMinutes, time.Minute),
	}
	return &operMng
}

func (op *OperMngr) AddImageProvider(imageProvider *ImageProvider) {
	op.imageProviders = append(op.imageProviders, imageProvider)
}

func (op *OperMngr) Start() error {
	if len(op.imageProviders) == 0 {
		return fmt.Errorf("no image providers found")
	}
	return nil
}

func (op *OperMngr) StartOperation(optype string, prompt string) (string, error) {
	if optype == "ydart" {
		op.logger.Info("Start direct provider operation")
		provider := op.getImageProvider()
		return op.startProviderOperation(provider, prompt, true)
	} else if optype == "old" {
		return op.startOldPictureOperation()
	}
	return op.startAutoOperation()

}

func (op *OperMngr) getImageProvider() *ImageProvider {
	if len(op.imageProviders) == 1 {
		op.logger.Debug("Get provider", "provider", (*op.imageProviders[0]).GetImageProviderForImageServerName())
		return op.imageProviders[0]
	}

	idx := rand.Intn(len(op.imageProviders))
	op.logger.Debug("Get provider", "provider", (*op.imageProviders[idx]).GetImageProviderForImageServerName())

	return op.imageProviders[idx]
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

func (op *OperMngr) startOldPictureOperation() (string, error) {
	op.logger.Info("Start old picture operation")
	file := op.dirManager.GetRandomFile()
	op.logger.Info("Start old picture operation", "file", file)
	operation := Operation{
		Id:         op.generateId(),
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
	op.logger.Info("Start ydart operation", "isDirectCall", isDirectCall)

	var externalId string
	var err error

	if prompt != "" {
		op.logger.Debug("Start ydart operation with prompt")
		externalId, err = (*provider).GenerateWithPrompt(strings.Trim(prompt, " "), isDirectCall)
	} else {
		externalId, err = (*provider).Generate(isDirectCall)
	}

	if err != nil {
		resultError := fmt.Errorf("error YdArt generate %v", err)
		op.logger.Error("Can not start operation", "error", resultError)
		return "", resultError
	}

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
	fileName := op.generateFileName(id)
	ydOperationResult, err := (*provider).GetImage(operation.(*Operation).ExternalId, fileName)
	if err != nil {
		return nil, err
	}

	if ydOperationResult {
		op.logger.Debug("Operation completed", "id", operation.(*Operation).Id, "fileName", fileName)
		completeOperation := operation.(*Operation)
		completeOperation.status = &OperStatus{Status: StatusDone, Error: ""}
		completeOperation.FileName = fileName
		op.completeOperations.SetDefault(id, completeOperation)
		op.pendingOperations.Delete(id)

		op.dirManager.AddFile(fileName)
	}

	return &OperStatus{Status: StatusPending}, nil
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

func (op *OperMngr) generateId() string {
	unixSeconds := time.Now().Unix()
	return "i" + strconv.Itoa(int(unixSeconds))
}

func (op *OperMngr) generateFileName(id string) string {
	unixSeconds := time.Now().Unix()
	return filepath.Join(op.directoryPath, "f"+strconv.Itoa(int(unixSeconds))+".jpeg")
}
