package promptmanager

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
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

type PromptMap map[int]PromptValue

type PromptManager struct {
	prompts PromptMap
	maxKeys int
	logger  *slog.Logger
	mutex   sync.Mutex
}

type Prompt struct {
	Idx      int     `json:"idx"`
	Prompt   string  `json:"prompt"`
	Negative *string `json:"negative,omitempty"` // Обратите внимание на указатель и `omitempty`
}

type PromptsData struct {
	Prompts []Prompt `json:"prompts"`
}

const (
	FILE_PATH_OPTIONS = "/data/prompts.json"
)

func NewPromptManager(logger *slog.Logger) (*PromptManager, error) {
	pm := &PromptManager{logger: logger, maxKeys: 10}

	promptsData, err := pm.readJSON()
	if err != nil {
		return nil, err
	}

	promptsToMap := pm.convertPromptsToMap(promptsData.Prompts)
	pm.prompts = promptsToMap
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
		return pm.prompts[1], nil
	}

	for i := 0; i < maxRetries; i++ {

		randomIndex := rand.Intn(keysCount) + 1 // +1, так как ключи начинаются с 1

		value, exists := pm.prompts[randomIndex]
		if exists {
			return value, nil
		}
	}
	pm.logger.Error("Failed to select an existing item after the maximum number of attempts", "maxAttempts", maxRetries)
	return PromptValue{}, fmt.Errorf("failed to select an existing item after the maximum number of attempts")
}

func (pm *PromptManager) GetPrompts() PromptMap {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	return copyPromptMap(pm.prompts)
}

func (pm *PromptManager) AddNewPrompt(newPrompt PromptValue) error {
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
	err := pm.writeJSON(&PromptsData{Prompts: prompts})
	if err != nil {
		pm.logger.Error("can not save new prompts into file", err.Error())
		return err
	}
	return nil
}

func (pm *PromptManager) readJSON() (*PromptsData, error) {
	// Проверяем, существует ли файл
	if _, err := os.Stat(FILE_PATH_OPTIONS); os.IsNotExist(err) {
		// Если файл не существует, вариант по умолчанию
		promptData := createDefaultPrompts()
		pm.writeJSON(promptData)
		return promptData, nil
	}

	plan, _ := os.ReadFile(FILE_PATH_OPTIONS)
	var d PromptsData
	err := json.Unmarshal(plan, &d)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (pm *PromptManager) writeJSON(d *PromptsData) error {
	jsonData, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		pm.logger.Error("Can not marshal", err)
		return fmt.Errorf("can not marshal: %w", err)
	}
	filename := FILE_PATH_OPTIONS
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
		pm.logger.Debug("Crate new prompts file", "filename", filename)
		fmt.Printf("Файл '%s' создан.\n", filename)
	} else {
		pm.logger.Debug("Rewrite prompts file", "filename", filename)
	}

	return nil
}

func (pm *PromptManager) convertPromptsToMap(prompts []Prompt) PromptMap {
	promptMap := make(map[int]PromptValue)
	for _, prompt := range prompts {
		if _, exists := promptMap[prompt.Idx]; exists {
			pm.logger.Error("Not unique idx", "idx", prompt.Idx)
			continue
		}
		promptMap[prompt.Idx] = PromptValue{
			Prompt:   prompt.Prompt,
			Negative: prompt.Negative,
		}
	}
	pm.logger.Debug("Converted", "count", len(promptMap))
	return promptMap
}

func (pm *PromptManager) existsPromptValue(prompt PromptValue) bool {
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
	copiedMap := make(map[int]PromptValue, len(originalMap))
	for key, value := range originalMap {
		// Копируем каждое значение в новую карту
		copiedMap[key] = value
	}
	return copiedMap
}

func createDefaultPrompts() *PromptsData {
	defaultPrompt := "test"
	return &PromptsData{
		Prompts: []Prompt{
			{
				Idx:    1,
				Prompt: defaultPrompt,
			},
		},
	}
}
