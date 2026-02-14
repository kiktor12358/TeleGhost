package appcore

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExportReseed создает ZIP-архив с частью базы netDb для оффлайн-ресидинга.
// Возвращает путь к созданному архиву.
func (a *AppCore) ExportReseed() (string, error) {
	// 1. Определяем путь к netDb
	// Предполагаем, что i2pd хранит данные в a.DataDir/i2pd/netDb или a.DataDir/netDb
	// Проверим оба варианта
	netDbPath := filepath.Join(a.DataDir, "i2pd", "netDb")
	if _, err := os.Stat(netDbPath); os.IsNotExist(err) {
		netDbPath = filepath.Join(a.DataDir, "netDb")
		if _, err := os.Stat(netDbPath); os.IsNotExist(err) {
			// На Android может быть dataDir/i2p/netDb
			netDbPath = filepath.Join(a.DataDir, "i2p", "netDb")
			if _, err := os.Stat(netDbPath); os.IsNotExist(err) {
				return "", fmt.Errorf("netDb folder not found in %s", a.DataDir)
			}
		}
	}

	// 2. Получаем список всех .dat файлов
	var files []string
	err := filepath.Walk(netDbPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".dat") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to list netDb files: %w", err)
	}

	if len(files) == 0 {
		return "", fmt.Errorf("netDb is empty")
	}

	// 3. Выбираем случайные 50-100 файлов
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })

	count := 100
	if len(files) < count {
		count = len(files)
	}
	selectedFiles := files[:count]

	// 4. Создаем ZIP архив во временной папке внутри DataDir (для Android)
	tmpDir := filepath.Join(a.DataDir, "tmp")
	os.MkdirAll(tmpDir, 0700)
	archiveName := fmt.Sprintf("i2p_reseed_%s.zip", time.Now().Format("20060102_150405"))
	archivePath := filepath.Join(tmpDir, archiveName)

	zipFile, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	for _, file := range selectedFiles {
		// Получаем относительный путь для сохранения структуры папок внутри архива (r/routerInfo.dat)
		relPath, err := filepath.Rel(netDbPath, file)
		if err != nil {
			relPath = filepath.Base(file)
		}

		f, err := os.Open(file)
		if err != nil {
			continue
		}

		w, err := writer.Create(relPath)
		if err != nil {
			f.Close()
			continue
		}

		if _, err := io.Copy(w, f); err != nil {
			f.Close()
			continue
		}
		f.Close()
	}

	return archivePath, nil
}

// ImportReseed распаковывает ZIP-архив в netDb пользователя.
func (a *AppCore) ImportReseed(zipPath string) error {
	// 1. Определяем путь к netDb (создаем, если нет)
	// Используем приоритетный путь: a.DataDir/i2pd/netDb (или тот, который существует)
	baseDir := filepath.Join(a.DataDir, "i2pd")
	netDbPath := filepath.Join(baseDir, "netDb")

	// Если папки нет, попробуем найти существующую или создадим структуру
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		// Если базовой папки i2pd нет, возможно это Android
		if _, err := os.Stat(filepath.Join(a.DataDir, "i2p")); err == nil {
			baseDir = filepath.Join(a.DataDir, "i2p")
			netDbPath = filepath.Join(baseDir, "netDb")
		} else {
			// Создаем дефолтную
			os.MkdirAll(netDbPath, 0755)
		}
	} else {
		// Base dir exists, check netDb
		os.MkdirAll(netDbPath, 0755)
	}

	// 2. Открываем ZIP
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	// 3. Распаковываем
	for _, file := range reader.File {
		// Защита от Zip Slip
		fpath := filepath.Join(netDbPath, file.Name)
		if !strings.HasPrefix(fpath, filepath.Clean(netDbPath)+string(os.PathSeparator)) {
			continue
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		if _, err := io.Copy(outFile, rc); err != nil {
			log.Printf("[AppCore] Failed to copy reseed file: %v", err)
		}

		outFile.Close()
		rc.Close()
	}

	// 4. Попытка Graceful Reload (опционально)
	// Если i2pd поддерживает SIGHUP или мы можем дернуть через SAM (нет).
	// i2pd обычно сам подхватывает файлы.
	// Мы можем просто уведомить пользователя.

	return nil
}
