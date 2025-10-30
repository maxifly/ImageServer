package promptmanager

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"imgserver/internal/pkg/templater"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"sync"
)

type PromptValue struct {
	Prompt   string
	Negative *string
}

func (p PromptValue) String() string {
	negative := "nil"
	if p.Negative != nil {
		negative = *p.Negative
	}
	return fmt.Sprintf("Prompt: %s, Negative: %s", p.Prompt, negative)
}

type PromptMap map[int]Prompt

type PromptManager struct {
	prompts            PromptMap
	globalPlaceholders map[string][]string
	templater          *templater.TemplateProcessor
	maxKeys            int
	logger             *slog.Logger
	mutex              sync.Mutex
}

type Prompt struct {
	Idx          int                 `yaml:"idx"`
	Prompt       string              `yaml:"prompt"`
	Negative     *string             `yaml:"negative,omitempty"` // Обратите внимание на указатель и `omitempty`
	Placeholders map[string][]string `yaml:"placeholders,omitempty"`
}

type PromptsData struct {
	Prompts            []Prompt            `yaml:"prompts"`
	GlobalPlaceholders map[string][]string `yaml:"global_placeholders,omitempty"`
}

const (
	FILE_PATH_OPTIONS         = "/data/prompts.yaml"
	FILE_PATH_EXAMPLE_OPTIONS = "/data/prompts_example.yaml"
)

func NewPromptManager(maxKeys int, logger *slog.Logger) (*PromptManager, error) {

	pm := &PromptManager{logger: logger, maxKeys: maxKeys, templater: templater.NewTemplateProcessor()}

	// Создать файл с примером
	pm.writeYaml(FILE_PATH_EXAMPLE_OPTIONS, createExamplePrompts())

	// Прочитать данные промптов
	promptsData, err := pm.readYaml()
	if err != nil {
		return nil, err
	}

	promptsToMap := pm.convertPromptsToMap(promptsData.Prompts)
	pm.prompts = promptsToMap
	pm.globalPlaceholders = promptsData.GlobalPlaceholders

	if !pm.validatePrompts(promptsData.Prompts) {
		pm.logger.Warn("Invalid prompts found")
	} else {
		pm.logger.Info("Prompts successfully validated")
	}

	pm.logger.Debug("Read saved prompts", "count", len(pm.prompts))
	return pm, nil
}

func (pm *PromptManager) GetRandomPrompt() (PromptValue, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	maxRetries := 100
	keysCount := len(pm.prompts)

	if keysCount == 0 {
		pm.logger.Error("No prompts available")
		return PromptValue{}, fmt.Errorf("no prompts available")
	}

	if keysCount == 1 {
		return pm.convertToPromptValue(pm.prompts[1]), nil
	}

	for i := 0; i < maxRetries; i++ {

		randomIndex := rand.Intn(keysCount) + 1 // +1, так как ключи начинаются с 1

		value, exists := pm.prompts[randomIndex]
		if exists {
			return pm.convertToPromptValue(value), nil
		}
	}
	pm.logger.Error("Failed to select an existing item after the maximum number of attempts", "maxAttempts", maxRetries)
	return PromptValue{}, fmt.Errorf("failed to select an existing item after the maximum number of attempts")
}

func (pm *PromptManager) convertToPromptValue(prompt Prompt) PromptValue {
	if !pm.templater.IsContainPlaceholders(prompt.Prompt) {
		return PromptValue{Prompt: prompt.Prompt, Negative: prompt.Negative}
	}

	positive1 := pm.templater.ReplacePlaceholders(prompt.Prompt, prompt.Placeholders)
	positive := pm.templater.ReplacePlaceholders(positive1, pm.globalPlaceholders)
	return PromptValue{Prompt: positive, Negative: prompt.Negative}
}

func (pm *PromptManager) AddNewPrompt(newPrompt Prompt) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.existsPromptValue(newPrompt) {
		pm.logger.Debug("New prompt already exists", "prompt", newPrompt)
		return fmt.Errorf("new prompt already exists")
	}

	// Создаем копию оригинальной карты
	transformedMap := make(PromptMap, len(pm.prompts))
	for key, value := range pm.prompts {
		transformedMap[key] = value
	}

	currentKeys := len(transformedMap)

	if currentKeys == pm.maxKeys {
		// Находим максимальный ключ
		maxKey := 0
		for key := range transformedMap {
			if key > maxKey {
				maxKey = key
			}
		}

		// Перемещаем ключи на одну позицию вниз
		newMap := make(PromptMap)
		for key, value := range transformedMap {
			newKey := key - 1
			if newKey > 0 {
				newMap[newKey] = value
			}
		}

		// Добавляем новый PromptValue под новым ключом
		newMap[maxKey] = newPrompt

		pm.prompts = newMap
	} else {
		// Находим следующий ключ после максимального
		maxKey := 0
		for key := range transformedMap {
			if key > maxKey {
				maxKey = key
			}
		}

		// Добавляем новый элемент с новым ключом
		transformedMap[maxKey+1] = newPrompt

		pm.prompts = transformedMap
	}

	pm.logger.Debug("Prompts count", "count", len(pm.prompts))
	prompts := convertMapToPrompts(pm.prompts)
	err := pm.writeYaml(FILE_PATH_OPTIONS, &PromptsData{Prompts: prompts, GlobalPlaceholders: pm.globalPlaceholders})
	if err != nil {
		pm.logger.Error("can not save new prompts into file", err.Error())
		return err
	}
	return nil
}

func (pm *PromptManager) readYaml() (*PromptsData, error) {
	// Проверяем, существует ли файл
	if _, err := os.Stat(FILE_PATH_OPTIONS); os.IsNotExist(err) {
		// Если файл не существует, вариант по умолчанию
		promptData := createDefaultPrompts()
		pm.writeYaml(FILE_PATH_OPTIONS, promptData)
		return promptData, nil
	}

	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var d PromptsData
	err := yaml.Unmarshal(plan, &d)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (pm *PromptManager) writeYaml(filename string, d *PromptsData) error {
	jsonData, err := yaml.Marshal(d)
	if err != nil {
		pm.logger.Error("Can not marshal", err)
		return fmt.Errorf("can not marshal: %w", err)
	}

	// Проверяем, существует ли файл
	fileExists := true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		fileExists = false
	}

	// Открываем файл для записи (создаем, если не существует)
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		pm.logger.Error("Can not open prompts file", err, "filename", filename)
		return fmt.Errorf("can not open prompts file '%s': %w", filename, err)
	}
	defer file.Close()

	// Записываем JSON в файл
	_, err = file.Write(jsonData)
	if err != nil {
		pm.logger.Error("Can not write file", err, "filename", filename)
		return fmt.Errorf("can not write file '%s': %w", filename, err)
	}

	// Добавляем символ новой строки в конец файла
	_, err = file.WriteString("\n")
	if err != nil {
		pm.logger.Error("Can not write file", err, "filename", filename)
		return fmt.Errorf("can not write file '%s': %w", filename, err)
	}

	if !fileExists {
		pm.logger.Debug("Create new prompts file", "filename", filename)
		fmt.Printf("Файл '%s' создан.\n", filename)
	} else {
		pm.logger.Debug("Rewrite prompts file", "filename", filename)
	}

	return nil
}

func (pm *PromptManager) convertPromptsToMap(prompts []Prompt) PromptMap {
	promptMap := make(map[int]Prompt)
	for _, prompt := range prompts {
		if _, exists := promptMap[prompt.Idx]; exists {
			pm.logger.Error("Not unique idx", "idx", prompt.Idx)
			continue
		}
		promptMap[prompt.Idx] = prompt
	}
	pm.logger.Debug("Converted", "count", len(promptMap))
	return promptMap
}

func (pm *PromptManager) existsPromptValue(prompt Prompt) bool {
	for _, value := range pm.prompts {
		// Сравниваем Prompt и Negative
		if value.Prompt == prompt.Prompt {
			if (value.Negative == nil && prompt.Negative == nil) ||
				(value.Negative != nil && prompt.Negative != nil && *value.Negative == *prompt.Negative) {
				return true
			}
		}
	}
	return false
}

func (pm *PromptManager) validatePrompts(prompts []Prompt) bool {
	result := true

	for _, prompt := range prompts {
		if !pm.validatePrompt(prompt) {
			result = false
		}
	}
	return result
}

func (pm *PromptManager) validatePrompt(prompt Prompt) bool {
	if !pm.templater.IsContainPlaceholders(prompt.Prompt) {
		return true
	}

	placeholders := unionMaps(pm.globalPlaceholders, prompt.Placeholders)
	result, missing := pm.templater.ValidatePlaceholders(prompt.Prompt, placeholders)

	if !result {
		pm.logger.Warn("Prompt with template not valid", "prompt idx", prompt.Idx, "invalid placeholders", strings.Join(missing, ", "))
		return false
	}

	return true
}

func convertMapToPrompts(promptMap PromptMap) []Prompt {
	prompts := make([]Prompt, 0, len(promptMap))
	for idx, promptValue := range promptMap {
		prompts = append(prompts, Prompt{
			Idx:      idx,
			Prompt:   promptValue.Prompt,
			Negative: promptValue.Negative,
		})
	}
	return prompts
}

func copyPromptMap(originalMap PromptMap) PromptMap {
	copiedMap := make(map[int]Prompt, len(originalMap))
	for key, value := range originalMap {
		// Копируем каждое значение в новую карту
		copiedMap[key] = value
	}
	return copiedMap
}

func createDefaultPrompts() *PromptsData {
	return &PromptsData{
		Prompts: []Prompt{
			{
				Idx:    1,
				Prompt: "test",
			},
		},
	}
}

// createExamplePrompts Создать файл с примером
func createExamplePrompts() *PromptsData {
	defaultPrompt := "test"
	placeholders := make(map[string][]string)
	placeholders["Fruits"] = []string{"яблоко", "спелое яблоко", "orange"}
	placeholders["CoLoRs"] = []string{"красный", "голубой", "зелёный презелёный"}

	return &PromptsData{
		Prompts: []Prompt{
			{
				Idx:    1,
				Prompt: defaultPrompt,
			},
		},

		GlobalPlaceholders: placeholders,
	}
}

func unionMaps(firstMap, secondMap map[string][]string) map[string][]string {
	// Создаем независимую копию первой карты
	copiedMap := make(map[string][]string)

	// Копируем все ключи и значения из первой карты
	if firstMap != nil {
		for key, values := range firstMap {
			// Создаем независимую копию слайса
			copiedValues := make([]string, len(values))
			copy(copiedValues, values)
			copiedMap[key] = copiedValues
		}
	}

	// Добавляем все значения из второй карты
	if secondMap != nil {
		for key, values := range secondMap {
			// Создаем независимую копию слайса
			copiedValues := make([]string, len(values))
			copy(copiedValues, values)
			copiedMap[key] = copiedValues
		}
	}

	return copiedMap

}
