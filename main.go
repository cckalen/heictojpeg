package main

import (
	"fmt"
	"image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/adrium/goheif"
)

const logFileName = "logs.txt"

func main() {
	fmt.Println("Starting the program...")

	currentDir, err := getCurrentDirectory()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	jpegDir := ensureJPEGDirectoryExists(currentDir)
	files, err := getFilesInDirectory(currentDir)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	logs := processFiles(currentDir, jpegDir, files)
	saveLogsToFile(jpegDir, logs)

	fmt.Println("Program completed!")
}

func getCurrentDirectory() (string, error) {
	fmt.Println("Fetching the current directory...")
	return os.Getwd()
}

func ensureJPEGDirectoryExists(dir string) string {
	jpegDir := filepath.Join(dir, "jpegs")
	if err := os.MkdirAll(jpegDir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}
	return jpegDir
}

func getFilesInDirectory(dir string) ([]os.DirEntry, error) {
	return os.ReadDir(dir)
}

func saveLogsToFile(jpegDir string, logs map[string][]string) {
	logFilePath := filepath.Join(jpegDir, logFileName)
	logFile, err := os.Create(logFilePath)
	if err != nil {
		log.Fatalf("Failed to create log file: %v", err)
	}
	defer logFile.Close()

	fmt.Println("Saving logs to logs.txt...")

	for key, logMessages := range logs {
		if key == "general" {
			continue
		}
		for _, logMessage := range logMessages {
			fmt.Fprintln(logFile, logMessage)
		}
	}

	// Now write the general logs at the end of the file.
	if generalLogs, ok := logs["general"]; ok {
		for _, logMessage := range generalLogs {
			fmt.Fprintln(logFile, logMessage)
		}
	}
}

func processFiles(currentDir, jpegDir string, files []os.DirEntry) map[string][]string {
	fmt.Println("Processing files...")
	startTime := time.Now()

	logs := make(map[string][]string)
	fileChan, logChan := setupWorkers(currentDir, jpegDir, len(files))

	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	aggregateLogs(logChan, logs, currentDir, jpegDir, startTime)

	return logs
}

func setupWorkers(currentDir, jpegDir string, filesCount int) (chan os.DirEntry, chan map[string]string) {
	fileChan := make(chan os.DirEntry, filesCount)
	logChan := make(chan map[string]string, filesCount)

	var wg sync.WaitGroup
	workerCount := runtime.NumCPU()
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(fileChan, logChan, currentDir, jpegDir, &wg)
	}

	go func() {
		wg.Wait()
		close(logChan)
	}()

	return fileChan, logChan
}

func worker(fileChan chan os.DirEntry, logChan chan map[string]string, currentDir, jpegDir string, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range fileChan {
		logChan <- processFile(file, currentDir, jpegDir)
	}
}

func processFile(file os.DirEntry, currentDir, jpegDir string) map[string]string {
	logEntry := make(map[string]string)
	ext := strings.ToLower(filepath.Ext(file.Name()))

	if ext == ".heic" {
		fmt.Printf("Processing file: %s\n", file.Name())
		err := convertFile(currentDir, file.Name(), jpegDir)
		if err != nil {
			logEntry[file.Name()] = fmt.Sprintf("error details: %s", err)
		} else {
			logEntry[file.Name()] = "converted successfully"
		}
	}

	return logEntry
}
func aggregateLogs(logChan chan map[string]string, logs map[string][]string, currentDir, jpegDir string, startTime time.Time) {
	var totalHEICSize, totalJPEGSize int64
	generalLogs := []string{} // Storing general logs here
	for logItem := range logChan {
		for k := range logItem {
			heicFilePath := filepath.Join(currentDir, k)
			jpgFilePath := getJPEGFilePath(jpegDir, k)

			heicSizeBytes := getFileSize(heicFilePath)
			jpgSizeBytes := getFileSize(jpgFilePath)

			totalHEICSize += heicSizeBytes
			totalJPEGSize += jpgSizeBytes

			heicSize := humanReadableFileSize(heicSizeBytes)
			jpgSize := humanReadableFileSize(jpgSizeBytes)

			logs[k] = append(logs[k], fmt.Sprintf("%s %s > Converted > jpegs/%s.jpg %s", k, heicSize, strings.TrimSuffix(k, filepath.Ext(k)), jpgSize))
		}
	}

	// Add general logs to the generalLogs slice
	totalDuration := time.Since(startTime)
	totalLogLines := len(logs)
	generalLogs = append(generalLogs, fmt.Sprintf("\n%v Files", totalLogLines))
	generalLogs = append(generalLogs, fmt.Sprintf("Total Time Taken==%v", totalDuration))
	generalLogs = append(generalLogs, fmt.Sprintf("Average Time Per File==%v", totalDuration/time.Duration(totalLogLines)))
	generalLogs = append(generalLogs, fmt.Sprintf("Total HEIC File Size==%s", humanReadableFileSize(totalHEICSize)))
	generalLogs = append(generalLogs, fmt.Sprintf("Total JPEG Folder Size==%s", humanReadableFileSize(totalJPEGSize)))

	// Add the generalLogs slice to the main logs map
	logs["general"] = generalLogs
}

func getJPEGFilePath(jpegDir, originalFileName string) string {
	return filepath.Join(jpegDir, strings.TrimSuffix(originalFileName, filepath.Ext(originalFileName))+".jpg")
}

func getFileSize(path string) int64 {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fileInfo.Size()
}

func convertFile(currentDir, inputFileName, jpegDir string) error {
	inputFilePath := filepath.Join(currentDir, inputFileName)
	outputFileName := strings.TrimSuffix(filepath.Base(inputFileName), filepath.Ext(inputFileName)) + ".jpg"
	outputFilePath := filepath.Join(jpegDir, outputFileName)
	return convertHeicToJpg(inputFilePath, outputFilePath)
}

func humanReadableFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func convertHeicToJpg(input, output string) error {
	fileInput, err := os.Open(input)
	if err != nil {
		return err
	}
	defer fileInput.Close()

	exif, err := goheif.ExtractExif(fileInput)
	if err != nil {
		return err
	}

	// Seek back to the beginning of the file for the next operation.
	fileInput.Seek(0, 0)

	img, err := goheif.Decode(fileInput)
	if err != nil {
		return err
	}

	fileOutput, err := os.OpenFile(output, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer fileOutput.Close()

	w, err := newWriterExif(fileOutput, exif)
	if err != nil {
		return err
	}

	return jpeg.Encode(w, img, nil)
}

type writerSkipper struct {
	w           io.Writer
	bytesToSkip int
}

func (w *writerSkipper) Write(data []byte) (int, error) {
	if w.bytesToSkip <= 0 {
		return w.w.Write(data)
	}

	dataLen := len(data)
	if dataLen < w.bytesToSkip {
		w.bytesToSkip -= dataLen
		return dataLen, nil
	}

	n, err := w.w.Write(data[w.bytesToSkip:])
	n += w.bytesToSkip
	w.bytesToSkip = 0
	return n, err
}

func newWriterExif(w io.Writer, exif []byte) (io.Writer, error) {
	writer := &writerSkipper{w, 2}
	soi := []byte{0xff, 0xd8}
	if _, err := w.Write(soi); err != nil {
		return nil, err
	}

	if exif != nil {
		app1Marker := 0xe1
		markerlen := 2 + len(exif)
		marker := []byte{0xff, uint8(app1Marker), uint8(markerlen >> 8), uint8(markerlen & 0xff)}
		if _, err := w.Write(marker); err != nil {
			return nil, err
		}

		if _, err := w.Write(exif); err != nil {
			return nil, err
		}
	}

	return writer, nil
}
