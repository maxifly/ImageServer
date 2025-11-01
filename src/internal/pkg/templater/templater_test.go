package templater

import (
	"reflect"
	"slices"
	"testing"
)

// TestTemplateProcessor_ReplacePlaceholders тестирует основную функцию замены плейсхолдеров
func TestTemplateProcessor_ReplacePlaceholders(t *testing.T) {
	tp := NewTemplateProcessor()

	tests := []struct {
		name     string
		template string
		values   map[string][]string
		expected []string
	}{
		{
			name:     "Простая замена",
			template: "Hello, [[name]]!",
			values:   map[string][]string{"name": {"World"}},
			expected: []string{"Hello, World!"},
		},
		{
			name:     "Множественные значения",
			template: "Hello, [[name]]!",
			values:   map[string][]string{"name": {"Bob", "Alice", "World"}},
			expected: []string{"Hello, Bob!", "Hello, Alice!", "Hello, World!"},
		},
		{
			name:     "Множественные плейсхолдеры",
			template: "Hello, [[name]]! Welcome to [[app_name]].",
			values:   map[string][]string{"name": {"Alice"}, "app_name": {"MiniMax"}},
			expected: []string{"Hello, Alice! Welcome to MiniMax."},
		},
		{
			name:     "Частичная замена",
			template: "Hello, [[name]]! Today is [[date]]",
			values:   map[string][]string{"name": {"Bob"}}, // date отсутствует
			expected: []string{"Hello, Bob! Today is [[date]]"},
		},
		{
			name:     "Пустая карта значений",
			template: "Hello, [[name]]!",
			values:   map[string][]string{},
			expected: []string{"Hello, [[name]]!"},
		},
		{
			name:     "Пустой список значений",
			template: "Hello, [[name]]!",
			values:   map[string][]string{"name": {}},
			expected: []string{"Hello, [[name]]!"},
		},
		{
			name:     "Плейсхолдеры не найдены",
			template: "Hello, World!",
			values:   map[string][]string{"name": {"Alice"}},
			expected: []string{"Hello, World!"},
		},
		{
			name:     "Лишние значения в карте",
			template: "Hello, [[name]]!",
			values:   map[string][]string{"name": {"Alice"}, "extra": {"value"}},
			expected: []string{"Hello, Alice!"},
		},
		{
			name:     "Специальные символы в значениях",
			template: "Path: [[path]]",
			values:   map[string][]string{"path": {"/home/user/documents/file.txt"}},
			expected: []string{"Path: /home/user/documents/file.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tp.ReplacePlaceholders(tt.template, tt.values)
			if !slices.Contains(tt.expected, result) {
				t.Errorf("Expected %s, got %s", tt.expected[0], result)
			}
		})
	}
}

// TestTemplateProcessor_ValidatePlaceholders тестирует валидацию плейсхолдеров
func TestTemplateProcessor_ValidatePlaceholders(t *testing.T) {
	tp := NewTemplateProcessor()

	tests := []struct {
		name            string
		template        string
		values          map[string][]string
		expectedMissing []string
		expectedIsValid bool
	}{
		{
			name:            "Все плейсхолдеры имеют значения",
			template:        "Hello, [[name]]! Welcome to [[app]].",
			values:          map[string][]string{"name": {"Alice"}, "app": {"MiniMax"}},
			expectedMissing: []string{},
			expectedIsValid: true,
		},
		{
			name:            "Отсутствующие плейсхолдеры",
			template:        "Hello, [[name]]! Today is [[date]].",
			values:          map[string][]string{"name": {"Bob"}}, // date отсутствует
			expectedMissing: []string{"date"},
			expectedIsValid: false,
		},
		{
			name:            "Лишние значения",
			template:        "Hello, [[name]]!",
			values:          map[string][]string{"name": {"Alice"}, "extra": {"value"}},
			expectedMissing: []string{},
			expectedIsValid: true,
		},
		{
			name:            "И отсутствующие, и лишние",
			template:        "Hello, [[name]]! Date: [[date]].",
			values:          map[string][]string{"name": {"Bob"}, "time": {"now"}},
			expectedMissing: []string{"date"},
			expectedIsValid: false,
		},
		{
			name:            "Плейсхолдер с пустым списком значений",
			template:        "Hello, [[name]]! Date: [[date]].",
			values:          map[string][]string{"name": {"Bob"}, "date": {}},
			expectedMissing: []string{"date"},
			expectedIsValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, missing := tp.ValidatePlaceholders(tt.template, tt.values)

			if !reflect.DeepEqual(missing, tt.expectedMissing) {
				t.Errorf("ValidatePlaceholders() missing = %v, want %v", missing, tt.expectedMissing)
			}

			if result != tt.expectedIsValid {
				t.Errorf("ValidatePlaceholders() isValid = %v, want %v", result, tt.expectedIsValid)
			}
		})
	}
}

// TestTemplateProcessor_EscapeLiteral тестирует экранирование литералов
func TestTemplateProcessor_EscapeLiteral(t *testing.T) {
	tp := NewTemplateProcessor()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Простой плейсхолдер",
			input:    "Используйте [[username]] для входа",
			expected: "Используйте [[[username]]] для входа",
		},
		{
			name:     "Множественные плейсхолдеры",
			input:    "Плейсхолдеры: [[a]] и [[b]]",
			expected: "Плейсхолдеры: [[[a]]] и [[[b]]]",
		},
		{
			name:     "Строки без плейсхолдеров",
			input:    "Обычный текст без плейсхолдеров",
			expected: "Обычный текст без плейсхолдеров",
		},
		{
			name:     "Пустая строка",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tp.EscapeLiteral(tt.input)
			if result != tt.expected {
				t.Errorf("EscapeLiteral() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestTemplateProcessor_UnescapeLiteral тестирует разэкранирование литералов
func TestTemplateProcessor_UnescapeLiteral(t *testing.T) {
	tp := NewTemplateProcessor()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Экранированный плейсхолдер",
			input:    "Используйте [[[username]]] для входа",
			expected: "Используйте [[username]] для входа",
		},
		{
			name:     "Множественные экранированные",
			input:    "Плейсхолдеры: [[[a]]] и [[[b]]]",
			expected: "Плейсхолдеры: [[a]] и [[b]]",
		},
		{
			name:     "Строки без экранирования",
			input:    "Обычный текст без плейсхолдеров",
			expected: "Обычный текст без плейсхолдеров",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tp.UnescapeLiteral(tt.input)
			if result != tt.expected {
				t.Errorf("UnescapeLiteral() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// BenchmarkReplacePlaceholders тестирует производительность функции замены
func BenchmarkReplacePlaceholders(b *testing.B) {
	tp := NewTemplateProcessor()
	template := "Привет, [[user_name]]! Добро пожаловать в [[app_name]]. Текущее время: [[current_time]]."
	values := map[string][]string{
		"user_name":    {"Алексей Иванов", "Петр Петров"},
		"app_name":     {"MiniMax Platform"},
		"current_time": {"2025-10-28T04:26:52Z"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tp.ReplacePlaceholders(template, values)
	}
}

// TestNewTemplateProcessor тестирует создание нового процессора
func TestNewTemplateProcessor(t *testing.T) {
	tp := NewTemplateProcessor()
	if tp == nil {
		t.Error("NewTemplateProcessor() вернул nil")
	}
	if tp.placeholderPattern == nil {
		t.Error("placeholderPattern равен nil")
	}
}

// TestPlaceholderPattern тестирует корректность регулярного выражения
func TestPlaceholderPattern(t *testing.T) {
	tp := NewTemplateProcessor()

	tests := []struct {
		name     string
		input    []string
		expected bool
	}{
		{
			name:     "Валидные плейсхолдеры",
			input:    []string{"[[user_name]]", " [[user-id]] ", "  [[APP_VERSION_2_0]]"},
			expected: true,
		},
		{
			name:     "Невалидные плейсхолдеры (пробелы)",
			input:    []string{"[[user name]]"}, // пробелы не поддерживаются
			expected: false,
		},
		{
			name:     "Невалидные плейсхолдеры (спецсимволы)",
			input:    []string{"[[user@name]]", " [[user.name]]"}, // @ и . не поддерживаются
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range tt.input {
				result := tp.IsContainPlaceholders(v)
				if !reflect.DeepEqual(result, tt.expected) {
					t.Errorf("Извлечение плейсхолдеров: ответ %v, вместо %v для %v", result, tt.expected, v)
				}
			}

		})
	}
}
