package promptmanager

import (
	"context"
	"fmt"
	"imgserver/internal/pkg/dbase"
	"imgserver/internal/pkg/templater"
	"imgserver/internal/pkg/utils"
	"log/slog"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
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

type PromptMap map[string]Prompt

type PromptManager struct {
	prompts            PromptMap
	promptKeys         []string
	globalPlaceholders map[string][]string
	templater          *templater.TemplateProcessor
	maxKeys            int
	logger             *slog.Logger
	mutex              sync.Mutex
	rng                *rand.Rand
	statisticDao       *dbase.StatisticDao
}

type Prompt struct {
	Code         string              `yaml:"code,omitempty"`
	Idx          *int                `yaml:"idx,omitempty"`
	Prompt       string              `yaml:"prompt"`
	Negative     *string             `yaml:"negative,omitempty"`
	Placeholders map[string][]string `yaml:"placeholders,omitempty"`
}

type PromptsData struct {
	Prompts            []Prompt            `yaml:"prompts"`
	GlobalPlaceholders map[string][]string `yaml:"global_placeholders,omitempty"`
}

type PromptStatistic struct {
	Code   string
	UseCnt int64
}

const (
	FILE_PATH_OPTIONS         = "/data/prompts.yaml"
	FILE_PATH_EXAMPLE_OPTIONS = "/data/prompts_example.yaml"
)

func NewPromptManager(maxKeys int, statisticDao *dbase.StatisticDao, logger *slog.Logger) (*PromptManager, error) {

	pm := &PromptManager{
		logger:       logger,
		maxKeys:      maxKeys,
		templater:    templater.NewTemplateProcessor(),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		statisticDao: statisticDao,
	}

	// Создать файл с примером
	pm.writeYaml(FILE_PATH_EXAMPLE_OPTIONS, createExamplePrompts())

	// Прочитать данные промптов
	promptsData, err := pm.readYaml()
	if err != nil {
		return nil, err
	}

	promptsToMap := pm.convertPromptsToMap(promptsData.Prompts)
	pm.prompts = promptsToMap
	pm.promptKeys = utils.GetSortedKeys(promptsToMap)
	pm.globalPlaceholders = promptsData.GlobalPlaceholders

	if !pm.validatePrompts(promptsData.Prompts) {
		pm.logger.Warn("Invalid prompts found")
	} else {
		pm.logger.Info("Prompts successfully validated")
	}

	pm.logger.Debug("Read saved prompts", "count", len(pm.prompts))
	return pm, nil
}

func (pm *PromptManager) GetPromptByCode(code string) (Prompt, bool) {
	value, exists := pm.prompts[code]
	if !exists {
		return Prompt{}, false
	}

	return value, true

}

func (pm *PromptManager) GetRandomPromptValue() (PromptValue, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	maxRetries := 100
	keysCount := len(pm.promptKeys)

	if keysCount == 0 {
		pm.logger.Error("No prompts available")
		return PromptValue{}, fmt.Errorf("no prompts available")
	}

	if keysCount == 1 {
		return pm.GetPromptValue(pm.prompts[pm.promptKeys[1]]), nil
	}

	for i := 0; i < maxRetries; i++ {

		randomIndex := pm.rng.Intn(keysCount) + 1 // +1, так как ключи начинаются с 1

		value, exists := pm.prompts[pm.promptKeys[randomIndex]]
		if exists {

			return pm.GetPromptValue(value), nil
		}
	}
	pm.logger.Error("Failed to select an existing item after the maximum number of attempts", "maxAttempts", maxRetries)
	return PromptValue{}, fmt.Errorf("failed to select an existing item after the maximum number of attempts")
}

func (pm *PromptManager) GetPromptValue(prompt Prompt) PromptValue {

	//Increment counter
	_, err := pm.statisticDao.Increment(context.Background(), prompt.Code)
	if err != nil {
		pm.logger.Error("Can not save statistic", "error", err)
	}

	if !pm.templater.IsContainPlaceholders(prompt.Prompt) {
		return PromptValue{Prompt: prompt.Prompt, Negative: prompt.Negative}
	}

	positive1 := pm.templater.ReplacePlaceholders(prompt.Prompt, prompt.Placeholders)
	positive := pm.templater.ReplacePlaceholders(positive1, pm.globalPlaceholders)
	return PromptValue{Prompt: positive, Negative: prompt.Negative}
}

func (pm *PromptManager) AddGlobalPlaceholder(name string, values []string) error {

	_, ok := pm.globalPlaceholders[name]
	if ok {
		pm.logger.Error("Global placeholder already exists", "name", name)
		return fmt.Errorf("global placeholder already exists")
	}

	unique := getUniqueValues(values)
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.globalPlaceholders[name] = unique
	err := pm.saveFile()
	if err != nil {
		return err
	}

	return nil

}

func (pm *PromptManager) DeleteGlobalPlaceholder(name string) error {

	_, ok := pm.globalPlaceholders[name]
	if !ok {
		pm.logger.Error("Global placeholder not exists", "name", name)
		return fmt.Errorf("global placeholder not exists")
	}

	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for _, prompt := range pm.prompts {
		if pm.isPromptUseGlobalPlaceholder(&prompt, name) {
			pm.logger.Error("Can not delete used global placeholder", "name", name)
			return fmt.Errorf("global placeholder used by prompt %d", prompt.Idx)
		}
	}

	delete(pm.globalPlaceholders, name)
	err := pm.saveFile()
	if err != nil {
		return err
	}

	return nil
}

func (pm *PromptManager) isPromptUseGlobalPlaceholder(prompt *Prompt, globalPlaceholderName string) bool {
	if !pm.templater.IsContainPlaceholders(prompt.Prompt) {
		return false
	}

	if prompt.Placeholders != nil {
		_, ok := prompt.Placeholders[globalPlaceholderName]
		if ok {
			return false
		}
	}

	placeholders := pm.templater.ExtractPlaceholders(prompt.Prompt)

	for _, placeholder := range placeholders {
		if globalPlaceholderName == placeholder {
			return true
		}
	}

	return false

}

func (pm *PromptManager) ChangeGlobalPlaceholder(name string, values []string) error {

	_, ok := pm.globalPlaceholders[name]
	if !ok {
		pm.logger.Error("Global placeholder not exists", "name", name)
		return fmt.Errorf("global placeholder not exists")
	}

	unique := getUniqueValues(values)
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	pm.globalPlaceholders[name] = unique
	err := pm.saveFile()
	if err != nil {
		return err
	}

	return nil

}

func (pm *PromptManager) GetPlaceholderValuesById(name string) ([]string, bool) {
	values, ok := pm.globalPlaceholders[name]
	if !ok {
		pm.logger.Error("Global placeholder not exists", "name", name)
		return nil, false
	}

	return values, true
}

func (pm *PromptManager) AddNewPrompt(newPrompt Prompt) (string, error) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.existsPromptValue(newPrompt) {
		pm.logger.Debug("New prompt already exists", "prompt", newPrompt)
		return "", fmt.Errorf("new prompt already exists")
	}

	if !pm.validatePrompt(newPrompt) {
		pm.logger.Error("Prompt is not valid")
		return "", fmt.Errorf("prompt is non valid")
	}

	// Создаем копию оригинальной карты
	transformedMap := make(PromptMap, len(pm.prompts))
	for key, value := range pm.prompts {
		transformedMap[key] = value
	}

	transformedMap[newPrompt.Code] = newPrompt
	newKeys := utils.GetSortedKeys(transformedMap)

	pm.prompts = transformedMap
	pm.promptKeys = newKeys

	pm.logger.Debug("Prompts count", "count", len(pm.prompts))

	err := pm.saveFile()
	if err != nil {
		pm.logger.Error("can not save new prompts into file", "error", err.Error())
		return "", err
	}
	return newPrompt.Code, nil
}

func (pm *PromptManager) saveFile() error {
	pm.logger.Debug("Save prompts into file", "count", len(pm.prompts))
	prompts := convertMapToPrompts(pm.prompts)
	err := pm.writeYaml(FILE_PATH_OPTIONS, &PromptsData{Prompts: prompts, GlobalPlaceholders: pm.globalPlaceholders})
	if err != nil {
		pm.logger.Error("can not save prompts into file", err.Error())
		return err
	}

	return nil
}

func (pm *PromptManager) ChangePrompt(newPrompt Prompt) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.existsPromptValue(newPrompt) {
		pm.logger.Warn("Prompt with this data already exists", "prompt", newPrompt)
		return fmt.Errorf("prompt with this data already exists")
	}

	if !pm.validatePrompt(newPrompt) {
		pm.logger.Error("Prompt is not valid")
		return fmt.Errorf("prompt is non valid")
	}

	_, ok := pm.prompts[newPrompt.Code]
	if !ok {
		pm.logger.Error("Prompt with this code does not exist", "prompt", newPrompt)
		return fmt.Errorf("prompt with code does not exist")
	}

	pm.prompts[newPrompt.Code] = newPrompt

	err := pm.saveFile()
	if err != nil {
		pm.logger.Error("Can not save prompts into file", "error", err.Error())
		return fmt.Errorf("can not save prompts into file. %v", err)
	}
	return nil
}

func (pm *PromptManager) DeletePrompt(code string) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	_, ok := pm.prompts[code]
	if !ok {
		pm.logger.Error("Prompt with this code does not exist", "code", code)
		return fmt.Errorf("prompt with this code does not exist")
	}

	// Создаем копию оригинальной карты
	transformedMap := make(PromptMap, len(pm.prompts))
	for key, value := range pm.prompts {
		transformedMap[key] = value
	}
	delete(transformedMap, code)
	newKeys := utils.GetSortedKeys(transformedMap)

	pm.prompts = transformedMap
	pm.promptKeys = newKeys

	err := pm.statisticDao.Delete(context.Background(), code)
	if err != nil {
		pm.logger.Error("Can not delete prompt statistic", "error", err.Error())
	}

	err = pm.saveFile()
	if err != nil {
		pm.logger.Error("can not save prompts into file", "error", err.Error())
		return fmt.Errorf("can not save prompts into file. %v", err)
	}

	return nil
}

func (pm *PromptManager) GetPromptsData() *PromptsData {
	prompts := convertMapToPrompts(pm.prompts)
	return &PromptsData{Prompts: prompts, GlobalPlaceholders: pm.globalPlaceholders}
}

func (pm *PromptManager) GetStatistic() map[string]PromptStatistic {
	var result map[string]PromptStatistic = make(map[string]PromptStatistic)
	all, err := pm.statisticDao.GetAll(context.Background())
	if err != nil {
		return result
	}

	for _, value := range all {
		result[value.Code] = PromptStatistic{
			Code:   value.Code,
			UseCnt: value.UseCnt,
		}
	}

	return result
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

	//TODO Это код миграции, в будущих версиях его надо убрать
	var isMigrate = false
	var result []Prompt
	for _, prmt := range d.Prompts {
		isPromtMigrate, newPrompt := migrationPrompt(prmt)
		if isPromtMigrate {
			isMigrate = true
		}
		result = append(result, newPrompt)
	}
	d.Prompts = result

	if isMigrate {
		err := pm.writeYaml(FILE_PATH_OPTIONS, &d)
		if err != nil {
			pm.logger.Error("Can not write file after migration", err.Error())
			return nil, err
		}
	}

	return &d, nil
}

func (pm *PromptManager) writeYaml(filename string, d *PromptsData) error {

	// Глубокая копия + сортировка
	sortedPrompts := deepCopyAndSortPrompts(d.Prompts)

	// Создаём новую структуру для записи (не модифицируем оригинал)
	dataToWrite := PromptsData{
		Prompts:            sortedPrompts,
		GlobalPlaceholders: d.GlobalPlaceholders, // map — передаётся по ссылке, но если не меняете — можно так
	}

	jsonData, err := yaml.Marshal(&dataToWrite)
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
	promptMap := make(map[string]Prompt)
	for _, prompt := range prompts {
		if _, exists := promptMap[prompt.Code]; exists {
			pm.logger.Error("Not unique idx", "idx", prompt.Idx)
			continue
		}
		promptMap[prompt.Code] = prompt
	}
	pm.logger.Debug("Converted", "count", len(promptMap))
	return promptMap
}

func (pm *PromptManager) existsPromptValue(prompt Prompt) bool {
	for _, value := range pm.prompts {
		if value.isEqual(&prompt) {
			pm.logger.Debug("equal", "p1", value, "p2", prompt)
			return true
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
	for _, promptValue := range promptMap {
		prompts = append(prompts, Prompt{
			Code:         promptValue.Code,
			Prompt:       promptValue.Prompt,
			Negative:     promptValue.Negative,
			Placeholders: promptValue.Placeholders,
		})
	}
	return prompts
}

func iddxToCode(idx *int) string {
	return "prmt_" + strconv.Itoa(*idx)
}

func migrationPrompt(prompt Prompt) (bool, Prompt) {
	if prompt.Code != "" {
		return false, prompt
	}

	return true, Prompt{
		Idx:          nil,
		Code:         iddxToCode(prompt.Idx),
		Prompt:       prompt.Prompt,
		Negative:     prompt.Negative,
		Placeholders: prompt.Placeholders,
	}
}

func createDefaultPrompts() *PromptsData {
	return &PromptsData{
		Prompts: []Prompt{
			{
				Code:   "prmt_1",
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
				Idx:    nil,
				Code:   "test1",
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

// deepCopyPrompt создаёт глубокую копию одного Prompt
func deepCopyPrompt(p Prompt) Prompt {
	copied := Prompt{
		Code:   p.Code,
		Prompt: p.Prompt,
	}

	// Копируем Negative, если не nil
	if p.Negative != nil {
		negCopy := *p.Negative
		copied.Negative = &negCopy
	}

	// Копируем Placeholders
	if p.Placeholders != nil {
		copied.Placeholders = make(map[string][]string, len(p.Placeholders))
		for k, v := range p.Placeholders {
			copied.Placeholders[k] = make([]string, len(v))
			copy(copied.Placeholders[k], v)
		}
	}

	return copied
}

// deepCopyAndSortPrompts делает глубокую копию и сортирует по Code
func deepCopyAndSortPrompts(prompts []Prompt) []Prompt {
	if prompts == nil {
		return nil
	}

	// Глубокая копия
	copied := make([]Prompt, len(prompts))
	for i, p := range prompts {
		copied[i] = deepCopyPrompt(p)
	}

	// Сортировка по Idx
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].Code < copied[j].Code
	})

	return copied
}

func getUniqueValues(values []string) []string {

	unique := make(map[string]interface{})
	for _, value := range values {
		unique[value] = nil
	}

	result := make([]string, 0, len(unique))

	for v, _ := range unique {
		result = append(result, v)
	}

	return result
}

func (p *Prompt) isEqual(prompt *Prompt) bool {
	if p.Prompt != prompt.Prompt {
		return false
	}

	if (p.Negative != nil && prompt.Negative == nil) ||
		(p.Negative == nil && prompt.Negative != nil) ||
		(p.Negative != nil && prompt.Negative != nil && p.Negative != prompt.Negative) {
		return false
	}

	if (p.Placeholders != nil && prompt.Placeholders == nil) ||
		(p.Placeholders == nil && prompt.Placeholders != nil) {
		return false
	}

	if p.Placeholders != nil && prompt.Placeholders != nil {

		if len(p.Placeholders) != len(prompt.Placeholders) {
			return false
		}

		for k, v := range p.Placeholders {

			pv, ok := prompt.Placeholders[k]

			if !ok {
				return false
			}

			if !equalStringSlices(v, pv) {
				return false
			}
		}

	}
	return true
}

func equalStringSlices(a, b []string) bool {

	setA := make(map[string]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}

	setB := make(map[string]struct{}, len(b))
	for _, s := range b {
		setB[s] = struct{}{}
	}

	if len(setA) != len(setB) {
		return false
	}

	for kb, _ := range setB {
		if _, exists := setA[kb]; !exists {
			return false
		}
	}

	return true
}
