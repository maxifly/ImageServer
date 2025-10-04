package appimageserver

import (
	"encoding/json"
	"fmt"
	"github.com/natefinch/lumberjack"
	"imgserver/internal/pkg/mylogger"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/rest"
	"log/slog"
	"os"
)

const FILE_PATH_OPTIONS = "/data/options.json"

type ImgSrv struct {
	options ApplOptions
	logger  *slog.Logger
	restObj *rest.Rest
}

type ApplOptions struct {
	LogLevel string `json:"log_level"`
}

func NewImgSrv(port string) *ImgSrv {

	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Options: %v", err))
	}

	// Настройка обработчика для записи в файл с ротацией
	fileLogger := &lumberjack.Logger{
		Filename:   "app.log", // Имя файла логов
		MaxSize:    100,       // Максимальный размер файла в мегабайтах
		MaxBackups: 3,         // Максимальное количество старых файлов для хранения
		MaxAge:     28,        // Максимальный возраст файла в днях
		Compress:   true,      // Сжимать ли старые файлы в формате gzip
	}

	var logLevel = slog.LevelInfo

	if options.LogLevel == "DEBUG" {
		logLevel = slog.LevelDebug
	} else if options.LogLevel == "INFO" {
		logLevel = slog.LevelInfo
	} else if options.LogLevel == "WARNING" {
		logLevel = slog.LevelWarn
	} else {
		logLevel = slog.LevelError
	}

	fileHandler := slog.NewTextHandler(fileLogger, &slog.HandlerOptions{
		Level: logLevel, AddSource: true,
	})

	// Настройка обработчика для вывода на экран
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel, AddSource: true,
	})

	// Объединение обработчиков в группу
	group := mylogger.NewMultiHandler(fileHandler, consoleHandler)

	// Создание логгера с группой обработчиков
	logger := slog.New(group)

	logger.Info("Application started", slog.String("status", "OK"))
	logger.Debug("This is a debug message", slog.String("detail", "additional info"))
	logger.Warn("This is a warning message")
	logger.Error("This is an error message", slog.String("error", "something went wrong"))

	//logFormat := log.Ldate | log.Ltime | log.Lshortfile

	//debugLog := log.New(mylogger.NewNullWriter(), "DEBUG\t", logFormat)
	//infoLog := log.New(mylogger.NewNullWriter(), "INFO\t", logFormat)
	//errorLog := log.New(os.Stderr, "ERROR\t", logFormat)
	//isDebudDisable := true

	//scheduleLogLevel := gocron.LogLevelWarn

	// Test log messages

	//options, err := readOptions()
	//if err != nil {
	//	panic(fmt.Sprintf("Can not read Options: %v", err))
	//}
	//
	//if options.LogLevel == "DEBUG" {
	//	debugLog = log.New(os.Stdout, "DEBUG\t", logFormat)
	//	infoLog = log.New(os.Stdout, "INFO\t", logFormat)
	//	isDebudDisable = false
	//
	//}
	//if options.LogLevel == "INFO" {
	//	infoLog = log.New(os.Stdout, "INFO\t", logFormat)
	//}
	//
	//debugLog.Println("hello")
	//infoLog.Println("hello")
	//errorLog.Println("hello")
	//
	//// Инициализируем новую структуру с зависимостями приложения.
	//logger := mylogger.New(errorLog, infoLog, debugLog)
	//if isDebudDisable {
	//	logger.DisableDebug()
	//}

	operMng := opermanager.NewOperMngr()

	restObj, err := rest.NewRest(port, logger, operMng)
	if err != nil {
		logger.Error("Error create Rest %v", err)
		panic(fmt.Sprintf("error create Rest %v", err))
	}

	return &ImgSrv{
		options: options,
		logger:  logger,
		restObj: restObj,
	}
}
func (app *ImgSrv) Start() {
	err := app.restObj.Start()
	app.logger.Error("Error start rest", err)
}

func (app *ImgSrv) Stop() {}

func readOptions() (ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data ApplOptions
	err := json.Unmarshal(plan, &data)
	return data, err
}
