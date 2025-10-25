package appimageserver

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-co-op/gocron/v2"
	"github.com/natefinch/lumberjack"
	"gopkg.in/yaml.v3"
	"imgserver/internal/pkg/dirmanager"
	"imgserver/internal/pkg/mylogger"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/rest"
	"imgserver/internal/pkg/ydart"
	"log/slog"
	"os"
	"time"
)

const (
	FILE_PATH_OPTIONS                    = "/data/options.yml"
	checkPendingOperationScheduleDefault = "* * * * *"
	scanImageFolderScheduleDefault       = "0 0 * * *"
	imageLimitMinDefault                 = 1000
	imageLimitMaxDefault                 = 2000
	imageHeightDefault                   = 480
	imageWeightDefault                   = 320
)

type ImgSrv struct {
	options          ApplOptions
	logger           *slog.Logger
	restObj          *rest.Rest
	dirManager       *dirmanager.DirManager
	operManager      *opermanager.OperMngr
	scheduler        gocron.Scheduler
	scheduleLogLevel gocron.LogLevel
}

type ProvidersOptions struct {
	YdArtOptions *ydart.YdArtOptions `yaml:"ydArt"`
}
type IframeImageParameters struct {
	ImageWeight int `yaml:"image_weight"`
	ImageHeight int `yaml:"image_height"`
}

type ApplOptions struct {
	LogLevel                      string                   `yaml:"log_level"`
	ImagePath                     string                   `yaml:"image_path"`
	ImageLimitMin                 int                      `yaml:"image_amount_min"`
	ImageLimitMax                 int                      `yaml:"image_amount_max"`
	ImageGenerateThreshold        int                      `yaml:"image_generate_threshold"`
	CheckPendingOperationSchedule string                   `yaml:"check_pending_cron"`
	ScanImageFolderSchedule       string                   `yaml:"scan_image_cron"`
	IframeImageParameters         *IframeImageParameters   `yaml:"iframe_image_parameters"`
	SleepTimes                    []*opermanager.SleepTime `yaml:"sleep_time"`
	ProvidersOptions              *ProvidersOptions        `yaml:"providers"`
}

func NewImgSrv(port string) *ImgSrv {

	options, err := readOptions()
	if err != nil {
		panic(fmt.Sprintf("Can not read Options: %v", err))
	}

	// Настройка обработчика для записи в файл с ротацией
	fileLogger := &lumberjack.Logger{
		Filename:   "/log/app.log", // Имя файла логов
		MaxSize:    100,            // Максимальный размер файла в мегабайтах
		MaxBackups: 3,              // Максимальное количество старых файлов для хранения
		MaxAge:     28,             // Максимальный возраст файла в днях
		Compress:   true,           // Сжимать ли старые файлы в формате gzip
	}

	var logLevel = slog.LevelInfo
	scheduleLogLevel := gocron.LogLevelWarn

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

	logger.Error("This is not error. Current options", "options", spew.Sprintf("%+v", options))

	imageParameters := opermanager.ImageParameters{Height: options.IframeImageParameters.ImageHeight,
		Weight: options.IframeImageParameters.ImageWeight,
	}

	promptManager, err := promptmanager.NewPromptManager(logger)
	if err != nil {
		logger.Error("Error create PromptManager %v", err)
		panic(fmt.Sprintf("error create PromptManager %v", err))
	}

	ydArt := ydart.NewYdArt(promptManager, logger, options.ProvidersOptions.YdArtOptions)
	iYdArt := (opermanager.ImageProvider)(ydArt)
	err = iYdArt.SetImageParameters(&imageParameters)
	if err != nil {
		logger.Error("Error setting image parameters: %v", err)
		panic(fmt.Sprintf("error setting image parameters: %v", err))
	}

	dirManager, err := dirmanager.NewDirManager(options.ImagePath, options.ImageLimitMin, options.ImageLimitMax, logger)
	if err != nil {
		logger.Error("Error create DirManager %v", err)
		panic(fmt.Sprintf("error create DirManager %v", err))
	}

	operMng := opermanager.NewOperMngr(options.ImagePath, options.ImageGenerateThreshold,
		&imageParameters,
		options.SleepTimes, dirManager, logger)
	operMng.AddImageProvider(&iYdArt)

	restObj, err := rest.NewRest(port, logger, operMng, promptManager)
	if err != nil {
		logger.Error("Error create Rest %v", err)
		panic(fmt.Sprintf("error create Rest %v", err))
	}

	return &ImgSrv{
		options:          options,
		logger:           logger,
		restObj:          restObj,
		dirManager:       dirManager,
		operManager:      operMng,
		scheduleLogLevel: scheduleLogLevel,
	}
}
func (app *ImgSrv) Start() {
	err := app.dirManager.Start()
	if err != nil {
		app.logger.Error("Error start dirManager", err)
	}

	err = app.operManager.Start()
	if err != nil {
		app.logger.Error("Error start operManager", err)
	}

	scheduler, err := gocron.NewScheduler(gocron.WithLocation(time.UTC),
		gocron.WithLogger(
			gocron.NewLogger(app.scheduleLogLevel),
		))
	app.scheduler = scheduler

	// Проверка статуса невыполненных заданий
	_, err = app.scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			app.options.CheckPendingOperationSchedule,
			false,
		),
		gocron.NewTask(
			func() {
				app.operManager.CheckPendingOperations()
			},
		),
	)

	// Сканирование каталога с изображениями
	_, err = app.scheduler.NewJob(
		gocron.CronJob(
			// standard cron tab parsing
			app.options.ScanImageFolderSchedule,
			false,
		),
		gocron.NewTask(
			func() {
				err := app.dirManager.ReadFiles()
				if err != nil {
					app.logger.Error("Error when clear operation", "err", err)
				}
			},
		),
	)

	// Запуск планировщика в отдельной горутине
	go func() {
		app.scheduler.Start()
	}()

	err = app.restObj.Start()
	app.logger.Error("Error start rest", err)
}

func (app *ImgSrv) Stop() {
	_ = app.scheduler.Shutdown()
}

func readOptions() (ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var data ApplOptions
	//err := json.Unmarshal(plan, &data)
	err := yaml.Unmarshal(plan, &data)

	if data.CheckPendingOperationSchedule == "" {
		data.CheckPendingOperationSchedule = checkPendingOperationScheduleDefault
	}

	if data.ScanImageFolderSchedule == "" {
		data.ScanImageFolderSchedule = scanImageFolderScheduleDefault
	}

	if data.ImageLimitMax == 0 || data.ImageLimitMin == 0 {
		data.ImageLimitMin = imageLimitMinDefault
		data.ImageLimitMax = imageLimitMaxDefault
	}

	if data.IframeImageParameters == nil || data.IframeImageParameters.ImageWeight == 0 || data.IframeImageParameters.ImageHeight == 0 {
		data.IframeImageParameters = &IframeImageParameters{ImageWeight: imageWeightDefault, ImageHeight: imageHeightDefault}
	}

	if data.ImageLimitMin >= data.ImageLimitMax {
		panic("Option image_amount_min must be lower then image_amount_max")
	}
	return data, err
}
