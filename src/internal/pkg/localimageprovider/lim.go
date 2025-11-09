package localimageprovider

import (
	"fmt"
	"imgserver/internal/pkg/actioner"
	"imgserver/internal/pkg/dirmanager"
	"imgserver/internal/pkg/imageprocessor"
	"imgserver/internal/pkg/opermanager"
	"log/slog"
	"time"
)

const (
	ProviderCode = "Lim"
)

var _ opermanager.ImageProvider = (*Lim)(nil)

type Lim struct {
	options         *LimOptions
	actioner        *actioner.Actioner
	logger          *slog.Logger
	isEnabled       bool
	dm              *dirmanager.DirManager
	imageParameters *opermanager.ImageParameters
	ipr             *imageprocessor.Ipr
	properties      *opermanager.ProviderProperties
}

type LimOptions struct {
	ImageGenerateThreshold int    `yaml:"image_generate_threshold"`
	LocalImageFolder       string `yaml:"local_image_folder"`
}

func NewLim(logger *slog.Logger, options *LimOptions) (*Lim, error) {
	var dm *dirmanager.DirManager = nil

	if len(options.LocalImageFolder) > 0 {
		dm1, err := dirmanager.NewDirManagerWithoutCleanup(options.LocalImageFolder, logger)
		if err != nil {
			return nil, err
		}
		dm = dm1
	}

	return &Lim{
		options:   options,
		logger:    logger,
		actioner:  actioner.NewActioner(options.ImageGenerateThreshold, time.Minute),
		isEnabled: false,
		dm:        dm,
		ipr:       imageprocessor.NewIpr(logger),
		properties: &opermanager.ProviderProperties{
			IsCanWorkWithPrompt: false,
			IsNeedSaveOriginal:  false,
		},
	}, nil
}

func (lim *Lim) Start() error {
	//TODO implement me

	if lim.dm != nil {
		exists, err := lim.dm.IsDirectoryExists()
		if err != nil {
			return err
		}

		if !exists {
			return fmt.Errorf("lim directory does not exist")
		}

		err = lim.dm.Start()
		if err != nil {
			return err
		}

		err = lim.dm.ReadFiles()
		if err != nil {
			lim.logger.Error("Error read files from local directory", "error", err)
		}
	} else {
		lim.isEnabled = false
	}

	return nil
}

func (lim *Lim) GetImageProviderForImageServerName() string {
	return "LocalImageProvider"
}

func (lim *Lim) GetImageProviderCode() string {
	return ProviderCode
}

func (lim *Lim) Generate(isDirectCall bool) (string, error) {
	return "lim_operation_id", nil
}

func (lim *Lim) GenerateWithPrompt(prompt string, isDirectCall bool) (string, error) {
	return "lim_operation_id", fmt.Errorf("can not generate image by prompt")
}

func (lim *Lim) GetImage(operationId string, filename string, fileNameOriginalSize string) (bool, error) {
	sourceFile := lim.dm.GetRandomFile()
	err := lim.ipr.ProcessImageFromFile(filename, "", sourceFile, lim.imageParameters.Weight, lim.imageParameters.Height)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (lim *Lim) IsReadyForRequest() bool {
	return lim.isEnabled && lim.dm.GetFileCount() > 0
}

func (lim *Lim) SetImageParameters(parameters *opermanager.ImageParameters) error {
	lim.imageParameters = parameters
	return nil
}

func (lim *Lim) GetProperties() *opermanager.ProviderProperties {
	return lim.properties
}
