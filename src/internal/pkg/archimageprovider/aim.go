package archimageprovider

import (
	"fmt"
	"imgserver/internal/pkg/dirmanager"
	"imgserver/internal/pkg/imageprocessor"
	"imgserver/internal/pkg/metrics"
	"imgserver/internal/pkg/opermanager"
	"log/slog"
)

const (
	ProviderCode = "ArchIm"
)

var _ opermanager.ImageProvider = (*ArchIm)(nil)

type ArchIm struct {
	logger          *slog.Logger
	dm              *dirmanager.DirManager
	imageParameters *opermanager.ImageParameters
	ipr             *imageprocessor.Ipr
	properties      *opermanager.ProviderProperties
}

type LimOptions struct {
	ImageGenerateThreshold int    `yaml:"image_generate_threshold"`
	LocalImageFolder       string `yaml:"local_image_folder"`
}

func NewAim(dm *dirmanager.DirManager, imageParameters imageprocessor.ImageParameters, metrics *metrics.AppMetrics, logger *slog.Logger) (*ArchIm, error) {

	return &ArchIm{
		logger: logger,
		dm:     dm,
		ipr:    imageprocessor.NewIpr(imageParameters, logger),
		properties: &opermanager.ProviderProperties{
			IsCanWorkWithPrompt:    false,
			IsNeedSaveLocalFiles:   false,
			IsArchiveImageProvider: true,
		},
	}, nil
}

func (aim *ArchIm) Start() error {
	return nil
}

func (aim *ArchIm) GetImageProviderForImageServerName() string {
	return "ArchiveImageProvider"
}

func (aim *ArchIm) GetImageProviderCode() string {
	return ProviderCode
}

func (aim *ArchIm) Generate(isDirectCall bool) (string, error) {
	if aim.dm == nil {
		return "", fmt.Errorf("aim provider disabled: archive_image_folder is not set")
	}
	return "archim_operation_id", nil
}

func (aim *ArchIm) GenerateByParameters(prompt opermanager.GenerationParameters, isDirectCall bool) (string, error) {
	return "archim_operation_id", fmt.Errorf("can not generate image by prompt")
}

func (aim *ArchIm) GetImageSlice(operationId string) (bool, []byte, error) {
	if aim.dm == nil {
		return false, nil, fmt.Errorf("aim provider disabled: local_image_folder is not set")
	}
	sourceFile := aim.dm.GetRandomFile()

	jpg, err := aim.ipr.ConvertImageFileToJpg(sourceFile)
	if err != nil {
		aim.logger.Error("Error converting image to jpg", "error", err, "file", sourceFile)
		return true, nil, err
	}

	return true, jpg, nil
}

func (aim *ArchIm) IsReadyForRequest() bool {
	return true
}

func (aim *ArchIm) SetImageParameters(parameters *opermanager.ImageParameters) error {
	aim.imageParameters = parameters
	return nil
}

func (aim *ArchIm) GetProperties() *opermanager.ProviderProperties {
	return aim.properties
}

func (aim *ArchIm) Refresh() error {
	if aim.dm == nil {
		aim.logger.Info("aim disabled")
		return nil
	}
	aim.logger.Debug("Refresh local image provider")
	return aim.dm.ReadFiles()
}
