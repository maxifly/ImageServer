package opermanager

type ImageParameters struct {
	Height int
	Weight int
}

type ProviderProperties struct {
	IsCanWorkWithPrompt  bool
	IsNeedSaveLocalFiles bool
}

type ImageProvider interface {
	Start() error
	GetImageProviderForImageServerName() string
	GetImageProviderCode() string
	Generate(isDirectCall bool) (string, error)
	GenerateWithPrompt(prompt string, isDirectCall bool) (string, error)
	GetImage(operationId string, filename string, fileNameOriginalSize string) (bool, error)
	IsReadyForRequest() bool
	SetImageParameters(parameters *ImageParameters) error
	GetProperties() *ProviderProperties
}
