package opermanager

import (
	"fmt"
	"imgserver/internal/pkg/dirmanager"
	"imgserver/internal/pkg/ydart"
	"log/slog"
	"path/filepath"
	"strconv"
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
	ydArt              *ydart.YdArt
	dirManager         *dirmanager.DirManager
	idMutex            *IdMutex
	ydArtActioner      Actioner
}

//TODO Всё сделать

type OperStatus struct {
	Status Status
	Error  string
}

type Operation struct {
	Id         string
	ExternalId string
	FileName   string
	Type       generatorType
	status     *OperStatus
}

type Actioner struct {
	lastCallTime time.Time
	threshold    time.Duration
}

func NewOperMngr(directoryPath string, thresholdMinutes int,
	dirManager *dirmanager.DirManager, ydArt *ydart.YdArt,
	logger *slog.Logger) *OperMngr {

	pendingOperations := cache.New(1*time.Hour, 2*time.Hour)
	completeOperations := cache.New(1*time.Hour, 2*time.Hour)

	operMng := OperMngr{
		directoryPath:      directoryPath,
		pendingOperations:  pendingOperations,
		completeOperations: completeOperations,
		ydArt:              ydArt,
		dirManager:         dirManager,
		logger:             logger,
		idMutex:            NewIdMutex(),
		ydArtActioner: Actioner{lastCallTime: time.Time{},
			threshold: time.Duration(thresholdMinutes) * time.Minute},
	}
	return &operMng
}
func (op *OperMngr) StartOperation(optype string) (string, error) {
	if optype == "ydart" {
		return op.startYdArtOperation()
	} else if optype == "old" {
		return op.startOldPictureOperation()
	}
	return op.startAutoOperation()

}

func (op *OperMngr) startAutoOperation() (string, error) {
	op.logger.Info("Start auto operation")
	now := time.Now()
	if now.Sub(op.ydArtActioner.lastCallTime) >= op.ydArtActioner.threshold {
		op.logger.Debug("Threshold")
		// YdArt давно не вызывался
		operation, err := op.startYdArtOperation()
		if err != nil {
			return "", err
		}
		// Обновляем время последнего вызова
		op.ydArtActioner.lastCallTime = now
		return operation, nil
	} else {
		// Вызываем менеджер старых изображений
		return op.startOldPictureOperation()
	}
}

func (op *OperMngr) startOldPictureOperation() (string, error) {
	op.logger.Info("Start old picture operation")
	file := op.dirManager.GetRandomFile()

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
	op.completeOperations.SetDefault(operation.Id, operation)
	return operation.Id, nil

}

func (op *OperMngr) startYdArtOperation() (string, error) {
	op.logger.Info("Start yart operation")
	externalId, err := op.ydArt.Generate()
	if err != nil {
		resultError := fmt.Errorf("error YdArt generate %v", err)
		op.logger.Error("Can not start operation", "error", resultError)
		return "", resultError
	}

	operation := Operation{
		Id:         op.generateId(),
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

		return operation.(Operation).status, nil
	}
	fileName := op.generateFileName(id)
	ydOperationResult, err := op.ydArt.GetImage(operation.(Operation).ExternalId, fileName)
	if err != nil {
		return nil, err
	}

	if ydOperationResult {
		completeOperation := operation.(Operation)
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

	return operation.(Operation).FileName, nil

}

func (op *OperMngr) CheckPendingOperations() {

	count := op.pendingOperations.ItemCount()

	ids := make([]string, 0, count+10)
	for _, k := range op.pendingOperations.Items() {
		op.logger.Debug("Pending operation", "operationId", k.Object.(Operation).Id)
		ids = append(ids, k.Object.(Operation).Id)
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
	return filepath.Join(op.directoryPath, "f"+strconv.Itoa(int(unixSeconds)))
}
