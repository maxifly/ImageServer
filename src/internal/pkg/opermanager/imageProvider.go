package opermanager

type ImageProvider interface {
	GetImageProviderForImageServerName() string
	Generate(isDirectCall bool) (string, error)
	GenerateWithPrompt(prompt string, isDirectCall bool) (string, error)
	GetImage(operationId string, filename string) (bool, error)
	IsReadyForRequest() bool
}
