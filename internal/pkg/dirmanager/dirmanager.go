package dirmanager

import (
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// fileInfo хранит информацию о файле
type fileInfo struct {
	Name    string
	ModTime time.Time
}

// DirManager управляет файлами в заданном каталоге
type DirManager struct {
	directoryPath string
	fileList      []fileInfo
	limitMin      int
	limitMax      int
	fileMap       map[string]struct{}
	mutex         sync.Mutex
	logger        *slog.Logger
}

// NewDirManager создает новый экземпляр DirManager
func NewDirManager(path string, limitMin int, limitMax int, logger *slog.Logger) (*DirManager, error) {
	manager := &DirManager{
		directoryPath: path,
		limitMin:      limitMin,
		limitMax:      limitMax,
		logger:        logger,
		fileList:      []fileInfo{},
		fileMap:       make(map[string]struct{}),
	}

	return manager, nil
}
func (dm *DirManager) Start() error {
	go dm.ReadFiles()
	return nil
}

// ReadFiles читает все файлы из каталога и сохраняет их информацию в список и карту
func (dm *DirManager) ReadFiles() error {
	files, err := os.ReadDir(dm.directoryPath)
	if err != nil {
		return err
	}
	fileList := []fileInfo{}
	fileMap := make(map[string]struct{})

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".jpeg") {
			fullPath := filepath.Join(dm.directoryPath, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}
			fileList = append(fileList, fileInfo{
				Name:    fullPath,
				ModTime: info.ModTime(),
			})
			fileMap[fullPath] = struct{}{}
		}
	}

	dm.mutex.Lock()
	defer dm.mutex.Unlock()
	dm.fileList = fileList
	dm.fileMap = fileMap
	dm.logger.Debug("Read files ", "fileAmount", len(dm.fileList))
	return nil
}

// GetRandomFile возвращает случайное имя файла из списка
func (dm *DirManager) GetRandomFile() string {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.logger.Debug("Get random file", "fileAmount", len(dm.fileList))

	if len(dm.fileList) == 0 {
		return ""
	}
	index := rand.Intn(len(dm.fileList))
	return dm.fileList[index].Name
}

// AddFile добавляет новый файл в каталог и список, если он еще не существует
func (dm *DirManager) AddFile(filename string) error {
	dm.logger.Debug("Add file operation", "filename", filename)
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	// Проверяем расширение
	if !strings.HasSuffix(filename, ".jpeg") {
		return nil
	}

	fullPath := filename
	// Проверяем, существует ли файл в карте
	if _, exists := dm.fileMap[fullPath]; exists {
		return nil // Файл уже существует
	}
	// Если файл не существует, существует ли он в ОС
	// Обновляем список и карту
	dm.logger.Debug("Get file information", "filename", filename)
	fileInformation, err := os.Stat(fullPath)
	if err != nil {
		// Файла нет или это какой-то странный файл. Не надо добавлять
		dm.logger.Error("Get file information", "filename", filename, "error", err)
		return nil
	}

	dm.logger.Debug("Add file", "filename", fullPath)
	dm.fileList = append(dm.fileList, fileInfo{
		Name:    fullPath,
		ModTime: fileInformation.ModTime(),
	})
	dm.fileMap[fullPath] = struct{}{}
	// Проверяем лимит и очищаем, если необходимо
	if len(dm.fileList) > dm.limitMax {
		dm.logger.Debug("Need cleanup")
		dm.innerCleanUp()
	}
	return nil
}

// CleanUp удаляет наиболее старые файлы, если количество файлов превышает заданный предел
func (dm *DirManager) CleanUp() {
	dm.mutex.Lock()
	defer dm.mutex.Unlock()

	dm.innerCleanUp()
}

func (dm *DirManager) innerCleanUp() {

	if len(dm.fileList) <= dm.limitMax {
		return
	}
	// Сортируем файлы по времени изменения
	sort.Slice(dm.fileList, func(i, j int) bool {
		return dm.fileList[i].ModTime.Before(dm.fileList[j].ModTime)
	})

	// Удаляем лишние файлы
	for i := dm.limitMin; i < len(dm.fileList); i++ {
		// Удаляем из карты
		delete(dm.fileMap, dm.fileList[i].Name)
		// Удаляем файл
		err := os.Remove(dm.fileList[i].Name)
		if err != nil {
			dm.logger.Warn("Error when delete file", "file", dm.fileList[i].Name, "error", err.Error())
			continue
		}
	}
	// Обновляем список файлов
	dm.fileList = dm.fileList[:dm.limitMin]

	dm.logger.Debug("Cleanup", "length", len(dm.fileList))

}
