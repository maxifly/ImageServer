package appimageserver

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-co-op/gocron/v2"
	"github.com/natefinch/lumberjack"
	"gopkg.in/yaml.v3"
	"imgserver/internal/pkg/dirmanager"
	"imgserver/internal/pkg/imageprocessor"
	"imgserver/internal/pkg/localimageprovider"
	"imgserver/internal/pkg/metrics"
	"imgserver/internal/pkg/mylogger"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/rest"
	"imgserver/internal/pkg/utils"
	"imgserver/internal/pkg/ydart"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	FILE_PATH_OPTIONS                 = "/data/options.yml"
	refreshLocalImageProviderSchedule = "30 * * * *"
)

type ImgSrv struct {
	options          ApplOptions
	logger           *slog.Logger
	restObj          *rest.Rest
	dirManager       *dirmanager.DirManager
	operManager      *opermanager.OperMngr
	scheduler        gocron.Scheduler
	scheduleLogLevel gocron.LogLevel
	metrics          *metrics.AppMetrics
	lim              *localimageprovider.Lim
}

type ProvidersOptions struct {
	YdArtOptions *ydart.YdArtOptions            `yaml:"ydArt"`
	LimOptions   *localimageprovider.LimOptions `yaml:"lim"`
}
type IframeImageParameters struct {
	ImageWeight  int     `yaml:"image_weight"`
	ImageHeight  int     `yaml:"image_height"`
	FitThreshold float64 `yaml:"fit_threshold"`
}

type ApplOptions struct {
	LogLevel                      string                   `yaml:"log_level"`
	ImagePath                     string                   `yaml:"image_path"`
	ImageLimitMin                 int                      `yaml:"image_amount_min"`
	ImageLimitMax                 int                      `yaml:"image_amount_max"`
	ImageGenerateThreshold        int                      `yaml:"image_generate_threshold"`
	CheckPendingOperationSchedule string                   `yaml:"check_pending_cron"`
	ScanImageFolderSchedule       string                   `yaml:"scan_image_cron"`
	IframeImageParameters         IframeImageParameters    `yaml:"iframe_image_parameters"`
	SleepTimes                    []*opermanager.SleepTime `yaml:"sleep_time"`
	ProvidersOptions              *ProvidersOptions        `yaml:"providers"`
	DisabledProviders             []string                 `yaml:"disabled_providers"`
	PromptsAmount                 int                      `yaml:"prompts_amount"`
}

func defaultConfig() ApplOptions {
	ifp := IframeImageParameters{ImageWeight: 350, ImageHeight: 480, FitThreshold: 0.03}

	return ApplOptions{
		CheckPendingOperationSchedule: "* * * * *",
		ScanImageFolderSchedule:       "0 0 * * *",
		ImageLimitMin:                 1000,
		ImageLimitMax:                 2000,
		PromptsAmount:                 10,
		IframeImageParameters:         ifp,
	}
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

	logTimezoneConfiguration(logger)

	imageParameters := opermanager.ImageParameters{Height: options.IframeImageParameters.ImageHeight,
		Weight: options.IframeImageParameters.ImageWeight,
	}

	appMetrics := metrics.NewAppMetrics()

	promptManager, err := promptmanager.NewPromptManager(options.PromptsAmount, logger)
	if err != nil {
		logger.Error("Error create PromptManager %v", err)
		panic(fmt.Sprintf("error create PromptManager %v", err))
	}

	originalImagePath := filepath.Join(options.ImagePath, "original")

	dirManager, err := dirmanager.NewDirManager(originalImagePath, options.ImageLimitMin, options.ImageLimitMax, logger)
	if err != nil {
		logger.Error("Error create DirManager %v", err)
		panic(fmt.Sprintf("error create DirManager %v", err))
	}

	imgPrmt := imageprocessor.ImageParameters{
		ImageHeight:  options.IframeImageParameters.ImageHeight,
		ImageWeight:  options.IframeImageParameters.ImageWeight,
		FitThreshold: options.IframeImageParameters.FitThreshold,
	}

	operMng, err := opermanager.NewOperMngr(options.ImageGenerateThreshold,
		imgPrmt,
		options.SleepTimes, dirManager, appMetrics, logger)

	if err != nil {
		logger.Error("Error create OperManager %v", err)
		panic(fmt.Sprintf("error create OperManager %v", err))
	}

	imgsrv := ImgSrv{
		options:          options,
		logger:           logger,
		dirManager:       dirManager,
		operManager:      operMng,
		scheduleLogLevel: scheduleLogLevel,
		metrics:          appMetrics,
	}

	// Создание провайдеров

	if !utils.Contains(options.DisabledProviders, "ydArt") && options.ProvidersOptions.YdArtOptions != nil {
		ydArt := ydart.NewYdArt(imgPrmt, promptManager, logger, options.ProvidersOptions.YdArtOptions)
		iYdArt := (opermanager.ImageProvider)(ydArt)
		err = iYdArt.SetImageParameters(&imageParameters)
		if err != nil {
			logger.Error("Error setting image parameters for ydArt: %v", err)
			panic(fmt.Sprintf("error setting image parameters for ydArt: %v", err))
		}

		operMng.AddImageProvider(&iYdArt)
	}
	if !utils.Contains(options.DisabledProviders, "lim") && options.ProvidersOptions.LimOptions != nil {
		lim, err := localimageprovider.NewLim(imgPrmt, logger, options.ProvidersOptions.LimOptions)
		if err != nil {
			logger.Error("Error create lim provider: %v", err)
			panic(fmt.Sprintf("error create lim provider: %v", err))
		}
		iLim := (opermanager.ImageProvider)(lim)
		err = iLim.SetImageParameters(&imageParameters)
		if err != nil {
			logger.Error("Error setting image parameters for lim: %v", err)
			panic(fmt.Sprintf("error setting image parameters for lim: %v", err))
		}

		operMng.AddImageProvider(&iLim)
		imgsrv.lim = lim
	}

	restObj, err := rest.NewRest(port, logger, operMng, promptManager, appMetrics)
	if err != nil {
		logger.Error("Error create Rest %v", err)
		panic(fmt.Sprintf("error create Rest %v", err))
	}

	imgsrv.restObj = restObj

	return &imgsrv
}

func (app *ImgSrv) Start() {
	app.metrics.Start()
	err := app.dirManager.Start()
	if err != nil {
		app.logger.Error("Error start dirManager", "error", err)
	}

	err = app.operManager.Start()
	if err != nil {
		app.logger.Error("Error start operManager", "error", err)
		panic(fmt.Errorf("error start operManager: %v", err))
	}

	metrics.StartMetricsLogging(app.logger, 60*time.Minute)

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

	// Обновление данных провайдера локальных изображений
	if app.lim != nil {
		app.logger.Debug("Create refresh local image provider task")
		_, err = app.scheduler.NewJob(
			gocron.CronJob(
				// standard cron tab parsing
				refreshLocalImageProviderSchedule,
				false,
			),
			gocron.NewTask(
				func() {
					err := app.lim.Refresh()
					if err != nil {
						app.logger.Error("Error when refresh local image provider", "err", err)
					}
				},
			),
		)
	}

	// Запуск планировщика в отдельной горутине
	go func() {
		app.scheduler.Start()
	}()

	err = app.restObj.Start()
	app.logger.Error("Error start rest", "error", err)
}

func (app *ImgSrv) Stop() {
	_ = app.scheduler.Shutdown()
}

func readOptions() (ApplOptions, error) {
	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	data := defaultConfig()
	err := yaml.Unmarshal(plan, &data)

	if data.ImageLimitMin >= data.ImageLimitMax {
		panic("Option image_amount_min must be lower then image_amount_max")
	}

	return data, err
}

func logTimezoneConfiguration(logger *slog.Logger) {
	logger.Info("=== CHECKING THE TIME CONFIGURATION ===")

	info := getTimezoneInfo()

	// Основная информация
	logger.Info(fmt.Sprintf("= The final timezone: %s", info.SystemTZ))
	logger.Info(fmt.Sprintf("= Current time: %s", info.LocalTime))
	logger.Info(fmt.Sprintf("= UTC     time: %s", info.UTCTime))
	logger.Info(fmt.Sprintf("= Offset from UTC: %s", info.Offset))

	// Источник настроек
	logger.Info(fmt.Sprintf("= Sources of settings:"))

	if info.EnvironmentTZ != "" {
		logger.Info(fmt.Sprintf("=     TZ variable: %s", info.EnvironmentTZ))
	} else {
		logger.Info(fmt.Sprintf("=     TZ variable: not set"))
	}

	if info.HasLocaltime {
		logger.Info(fmt.Sprintf("=     /etc/localtime: mounted from the host"))
	} else {
		logger.Info(fmt.Sprintf("=     /etc/localtime: unavailable (using fallback)"))
	}

	logger.Info("=====================================")
}

type TimezoneInfo struct {
	EnvironmentTZ string
	SystemTZ      string
	LocalTime     string
	UTCTime       string
	Offset        string
	HasLocaltime  bool
	HasTZVariable bool
}

func getTimezoneInfo() TimezoneInfo {
	now := time.Now()

	info := TimezoneInfo{
		EnvironmentTZ: os.Getenv("TZ"),
		SystemTZ:      now.Location().String(),
		LocalTime:     now.Format("2006-01-02 15:04:05 MST"),
		UTCTime:       now.UTC().Format("2006-01-02 15:04:05 MST"),
		Offset:        now.Format("-0700"),
		HasTZVariable: os.Getenv("TZ") != "",
	}

	// Проверяем наличие /etc/localtime
	if _, err := os.Stat("/etc/localtime"); err == nil {
		info.HasLocaltime = true
	} else {
		info.HasLocaltime = false
	}

	return info
}
