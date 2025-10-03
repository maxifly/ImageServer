package appimageserver

import (
	"encoding/json"
	"fmt"
	"imgserver/internal/pkg/mylogger"
	"log"
	"os"
)

const FILE_PATH_OPTIONS = "/data/options.json"

type ImgSrv struct {
	options ApplOptions
	logger  *mylogger.Logger
}

type ApplOptions struct {
	LogLevel string `json:"log_level"`
}

func NewImgSrv(port string) *ImgSrv {
	logFormat := log.Ldate | log.Ltime | log.Lshortfile

	debugLog := log.New(mylogger.NewNullWriter(), "DEBUG\t", logFormat)
	infoLog := log.New(mylogger.NewNullWriter(), "INFO\t", logFormat)
	errorLog := log.New(os.Stderr, "ERROR\t", logFormat)
	isDebudDisable := true

	//scheduleLogLevel := gocron.LogLevelWarn

	// Test log messages

	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Options: %v", err))
	}

	if options.LogLevel == "DEBUG" {
		debugLog = log.New(os.Stdout, "DEBUG\t", logFormat)
		infoLog = log.New(os.Stdout, "INFO\t", logFormat)
		isDebudDisable = false

	}
	if options.LogLevel == "INFO" {
		infoLog = log.New(os.Stdout, "INFO\t", logFormat)
	}

	debugLog.Println("hello")
	infoLog.Println("hello")
	errorLog.Println("hello")

	// Инициализируем новую структуру с зависимостями приложения.
	logger := mylogger.New(errorLog, infoLog, debugLog)
	if isDebudDisable {
		logger.DisableDebug()
	}

	return &ImgSrv{
		options: options,
		logger:  logger,
	}
}
func (app *ImgSrv) Start() {}
func (app *ImgSrv) Stop()  {}

func readOptions() (ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data ApplOptions
	err := json.Unmarshal(plan, &data)
	return data, err
}
