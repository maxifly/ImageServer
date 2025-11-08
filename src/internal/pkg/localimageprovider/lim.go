package localimageprovider

import (
	"imgserver/internal/pkg/actioner"
	"imgserver/internal/pkg/opermanager"
	"log/slog"
	"time"
)

const (
	ProviderCode = "Lim"
)

var _ opermanager.ImageProvider = (*Lim)(nil)

type Lim struct {
	options   *LimOptions
	actioner  *actioner.Actioner
	logger    *slog.Logger
	isEnabled bool
}

type LimOptions struct {
	ImageGenerateThreshold int    `yaml:"image_generate_threshold"`
	LocalImageFolder       string `yaml:"local_image_folder"`
}

func NewLim(options *LimOptions, logger *slog.Logger) *Lim {
	return &Lim{
		options:   options,
		logger:    logger,
		actioner:  actioner.NewActioner(options.ImageGenerateThreshold, time.Minute),
		isEnabled: false,
	}
}

func (lim *Lim) Start() error {
	//TODO implement me
	panic("implement me")
}

func (lim *Lim) GetImageProviderForImageServerName() string {
	return "LocalImageProvider"
}

func (lim *Lim) GetImageProviderCode() string {
	return ProviderCode
}

func (lim *Lim) Generate(isDirectCall bool) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (lim *Lim) GenerateWithPrompt(prompt string, isDirectCall bool) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (lim *Lim) GetImage(operationId string, filename string, fileNameOriginalSize string) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (lim *Lim) IsReadyForRequest() bool {
	//TODO implement me
	panic("implement me")
}

func (lim *Lim) SetImageParameters(parameters *opermanager.ImageParameters) error {
	//TODO implement me
	panic("implement me")
}
