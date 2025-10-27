package templater

import (
	"math/rand"
	"regexp"
	"strings"
)

// TemplateProcessor структура для работы с плейсхолдерами
type TemplateProcessor struct {
	// Регулярное выражение для поиска плейсхолдеров формата [[placeholder_name]]
	placeholderPattern *regexp.Regexp
}

// NewTemplateProcessor создает новый экземпляр процессора шаблонов
func NewTemplateProcessor() *TemplateProcessor {
	// Паттерн: [[имя_плейсхолдера]], где имя может содержать буквы, цифры, подчеркивания и дефисы
	pattern := regexp.MustCompile(`\[\[([a-zA-Z0-9_-]+)\]\]`)
	return &TemplateProcessor{
		placeholderPattern: pattern,
	}
}

// ReplacePlaceholders заменяет плейсхолдеры в строке значениями из карты
// template - строка с плейсхолдерами формата [[placeholder_name]]
// values - карта, где ключ - это имя плейсхолдера, а значение - строка для замены
func (tp *TemplateProcessor) ReplacePlaceholders(template string, values map[string][]string) string {
	return tp.placeholderPattern.ReplaceAllStringFunc(template, func(match string) string {
		// Извлекаем имя плейсхолдера из match (убираем [[ и ]])
		placeholderName := match[2 : len(match)-2]

		// Ищем значение в карте и отдаём случайное значение из списка
		if value, exists := values[placeholderName]; exists {
			if value != nil {
				if len(value) == 1 {
					return value[0]
				} else if len(value) > 1 {
					return value[rand.Intn(len(value))]
				}
			}
		}

		// Если значение не найдено, возвращаем оригинальный плейсхолдер
		// Можно также вернуть пустую строку или ошибку в зависимости от требований
		return match
	})
}

func (tp *TemplateProcessor) ValidatePlaceholders(template string, values map[string][]string) (result bool, missing []string) {

	placeholders := tp.ExtractPlaceholders(template)
	missing = make([]string, 0)
	for _, ph := range placeholders {
		v, exists := values[ph]

		if !exists || v == nil || len(v) == 0 {
			missing = append(missing, ph)
		}
	}

	if len(missing) > 0 {
		result = false
	} else {
		result = true
	}

	return result, missing
}

// IsContainPlaceholders определяет есть ли в строке хотя бы один плейсхолдер
func (tp *TemplateProcessor) IsContainPlaceholders(template string) bool {
	return tp.placeholderPattern.MatchString(template)
}

// ExtractPlaceholders извлекает все имена плейсхолдеров из строки
func (tp *TemplateProcessor) ExtractPlaceholders(template string) []string {
	matches := tp.placeholderPattern.FindAllStringSubmatch(template, -1)
	placeholders := make([]string, 0, len(matches))

	for _, match := range matches {
		if len(match) > 1 {
			placeholders = append(placeholders, match[1])
		}
	}

	return placeholders
}

// EscapeLiteral экранирует строку, чтобы плейсхолдеры в ней не заменялись
func (tp *TemplateProcessor) EscapeLiteral(literal string) string {
	// Экранируем все [[ как [[[ и ]] как ]]]
	literal = strings.ReplaceAll(literal, "[[", "[[[")
	literal = strings.ReplaceAll(literal, "]]", "]]]")
	return literal
}

// UnescapeLiteral разэкранирует строку
func (tp *TemplateProcessor) UnescapeLiteral(literal string) string {
	literal = strings.ReplaceAll(literal, "[[[", "[[")
	literal = strings.ReplaceAll(literal, "]]]", "]]")
	return literal
}
