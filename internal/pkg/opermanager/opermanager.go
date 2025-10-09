package opermanager

import (
	"fmt"
	"imgserver/internal/pkg/ydart"
	"log/slog"
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
	pendingOperations  *cache.Cache
	completeOperations *cache.Cache
	logger             *slog.Logger
	ydArt              *ydart.YdArt
	idMutex            *IdMutex
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

func NewOperMngr(ydArt *ydart.YdArt, logger *slog.Logger) *OperMngr {
	pendingOperations := cache.New(1*time.Hour, 2*time.Hour)
	completeOperations := cache.New(1*time.Hour, 2*time.Hour)

	operMng := OperMngr{
		pendingOperations:  pendingOperations,
		completeOperations: completeOperations,
		ydArt:              ydArt,
		logger:             logger,
		idMutex:            NewIdMutex(),
	}
	return &operMng
}

func (op *OperMngr) StartOperation() (string, error) {
	// TODO Пока все операции, это YandexArt
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
	return "f" + strconv.Itoa(int(unixSeconds))
}
