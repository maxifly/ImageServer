package dirmanager

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirManager_IsDirectoryExists(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		createDir bool
		want      bool
	}{
		{"withoutMakeDir",
			"dir1",
			false,
			false,
		},
		{"withMakeDir",
			"dir2",
			true,
			true,
		},
	}
	Prepare()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			dm, err := NewDirManager(filepath.Join("tests", "dm", tt.dir), 1, 10, logger)

			if tt.createDir {
				dm.Start()
			}

			got, err := dm.IsDirectoryExists()
			if err != nil {
				t.Errorf("IsDirectoryExists() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("IsDirectoryExists() got = %v, want %v", got, tt.want)
			}
		})
	}
	Clear()
}

func TestDirManager_AddFile(t *testing.T) {
	//type args struct {
	//	filename string
	//}
	tests := []struct {
		name string
		//args    args
		fileType        string
		startFilesCount int
		wantFilesCount  int
	}{
		{
			"lessMinBound",
			"jpeg",
			2,
			3,
		},
		{
			"lessMaxBound",
			"jpeg",
			4,
			5,
		},
		{
			"greatestMaxBound",
			"jpeg",
			5,
			3,
		},
		{
			"unexpected file type",
			"jjj",
			5,
			6,
		},
	}

	Prepare()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug, AddSource: true,
	}))
	dirPath := filepath.Join("tests", "dm", "testDir")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			dm, err := NewDirManager(dirPath, 3, 5, logger)
			if err != nil {
				t.Errorf("Create dm error = %v", err)
				return
			}

			err = dm.Start()
			if err != nil {
				t.Errorf("Start dm error = %v", err)
				return
			}

			// Add start files
			files, err := createRandomFiles(dirPath, tt.startFilesCount, tt.fileType)
			if err != nil {
				t.Errorf("Can not create start files = %v", err)
				return
			}

			for _, file := range files {
				err := dm.AddFile(file)
				if err != nil {
					t.Errorf("Error add file to dm = %v", err)
					return
				}
			}

			lastFile := "last_file_name.jpeg"

			// Add last file
			_, err = createFileInDir(dirPath, lastFile)
			if err != nil {
				t.Errorf("Can not create last file = %v", err)
				return
			}

			err = dm.AddFile(filepath.Join(dirPath, lastFile))
			if err != nil {
				t.Errorf("Error add file to dm = %v", err)
				return
			}

			entries, _ := os.ReadDir(dirPath)

			if tt.wantFilesCount != len(entries) {
				t.Errorf("files in directory got = %v, want %v", len(entries), tt.wantFilesCount)
			}

			found := false
			for _, name := range entries {
				if name.Name() == lastFile {
					found = true
				}
			}

			if !found {
				t.Errorf("Last file not found in directory %v", entries)
			}

			removeAllContents(dirPath)

		})
	}
	Clear()
}

func Prepare() {
	os.Mkdir("tests", 0777)
	os.Mkdir("tests/dm", 0777)
}

func Clear() {
	removeAllContents("tests")
}

func removeAllContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func createFileInDir(dir, filename string) (string, error) {
	// Формируем полный путь к файлу
	fullPath := filepath.Join(dir, filename)

	// Создаём файл (если он уже существует — будет перезаписан как пустой)
	file, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	file.Close() // Важно: закрываем файл сразу после создания

	return fullPath, nil
}

func createRandomFiles(dir string, n int, ftype string) ([]string, error) {
	var paths []string
	for i := 0; i < n; i++ {
		time.Sleep(2 * time.Second)
		// Генерируем уникальное имя
		timestamp := time.Now().UnixMilli()
		filename := fmt.Sprintf("file-%d-%d.%s", i, timestamp, ftype)

		path, err := createFileInDir(dir, filename)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
