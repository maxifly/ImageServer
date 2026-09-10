package ydart

import (
	"fmt"
	"os"
	"sync"
)

// FileStorage инкапсулирует работу с файлами и синхронизацию
type FileStorage struct {
	mu sync.RWMutex // RWMutex позволяет параллельное чтение, но эксклюзивную запись
}

// NewFileStorage создает новый экземпляр хранилища
func NewFileStorage() *FileStorage {
	return &FileStorage{}
}

// SaveToFile записывает бинарные данные в файл (эксклюзивная блокировка)
func (fs *FileStorage) SaveToFile(filePath string, data []byte) error {
	// Блокируем запись эксклюзивно (ни чтение, ни запись не возможны)
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("не удалось записать файл %s: %w", filePath, err)
	}
	return nil
}

// LoadFromFile читает файл в бинарные данные (разделяемая блокировка)
func (fs *FileStorage) LoadFromFile(filePath string) ([]byte, error) {
	// Блокируем только для чтения (другие чтения могут идти параллельно)
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл %s: %w", filePath, err)
	}
	return data, nil
}
