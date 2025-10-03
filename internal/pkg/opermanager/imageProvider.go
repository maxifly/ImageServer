package opermanager

type ImageProvider interface {
	GetImageProviderForImageServerName() string
	Generate() (string, error)
	GenerateWithPrompt(prompt string) (string, error)
	GetImage(operationId string, filename string) (bool, error)
}
