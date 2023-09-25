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

func main() {
	fmt.Println("Starting the program...")
	run()
	fmt.Println("Program completed!")
}

func run() {
	fmt.Println("Fetching the current directory...")
	currentDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current directory: %v", err)
	}

	files, err := os.ReadDir(currentDir)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}

	startTime := time.Now()
	logs := make(map[string]string)
	jpegDir := filepath.Join(currentDir, "jpegs")

	// Ensure the /jpegs directory exists
	if err := os.MkdirAll(jpegDir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}

	fmt.Println("Processing files...")

	// Use buffered channels
	fileChan := make(chan os.DirEntry, len(files))
	logChan := make(chan map[string]string, len(files))

	var wg sync.WaitGroup

	// Set workerCount based on CPU cores
	workerCount := runtime.NumCPU()
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				fileLog := make(map[string]string)
				ext := strings.ToLower(filepath.Ext(file.Name()))
				if ext == ".heic" {
					fmt.Printf("Processing file: %s\n", file.Name())
					if err := convertFile(currentDir, file.Name(), jpegDir); err != nil {
						fileLog[file.Name()] = fmt.Sprintf("error details: %s", err)
					} else {
						fileLog[file.Name()] = "converted successfully"
					}
				}
				logChan <- fileLog
			}
		}()
	}

	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	go func() {
		wg.Wait()
		close(logChan)
	}()

	var totalHEICSize int64
	var totalJPEGSize int64

	for logItem := range logChan {
		for k := range logItem {
			heicFilePath := filepath.Join(currentDir, k)
			jpgFilePath := filepath.Join(jpegDir, strings.TrimSuffix(k, filepath.Ext(k))+".jpg")

			// Fetch sizes
			heicSizeBytes := getFileSize(heicFilePath)
			jpgSizeBytes := getFileSize(jpgFilePath)

			totalHEICSize += heicSizeBytes
			totalJPEGSize += jpgSizeBytes

			heicSize := humanReadableFileSize(heicSizeBytes)
			jpgSize := humanReadableFileSize(jpgSizeBytes)

			logs[k] = fmt.Sprintf("%s %s > Converted > jpegs/%s.jpg %s", k, heicSize, strings.TrimSuffix(k, filepath.Ext(k)), jpgSize)
		}
	}

	// Compute total duration and average duration per file
	totalDuration := time.Since(startTime)
	totalLogLines := float64(len(logs))
	totalMilliseconds := int(totalDuration.Milliseconds())
	seconds := totalMilliseconds / 1000
	milliseconds := totalMilliseconds % 1000

	averageDurationPerFileSeconds := seconds / len(logs)
	averageDurationPerFileMilliseconds := totalMilliseconds / len(logs) % 1000

	// Save logs to logs.txt
	logFilePath := filepath.Join(jpegDir, "logs.txt")
	logFile, err := os.Create(logFilePath)
	if err != nil {
		log.Fatalf("Failed to create log file: %v", err)
	}
	defer logFile.Close()

	fmt.Println("Saving logs to logs.txt...")
	for file, logMessage := range logs {
		fmt.Fprintf(logFile, "%s==%s\n", file, logMessage)
	}
	// Add total and average durations to the log file
	fmt.Fprintf(logFile, "\n%v Files \nTotal time taken: %ds %dms\n", totalLogLines, seconds, milliseconds)
	fmt.Fprintf(logFile, "Average time per file: %ds %dms\n", averageDurationPerFileSeconds, averageDurationPerFileMilliseconds)

	fmt.Fprintf(logFile, "\nTotal HEIC files size: %s\n", humanReadableFileSize(totalHEICSize))
	fmt.Fprintf(logFile, "Total JPEG folder size: %s\n", humanReadableFileSize(totalJPEGSize))

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
