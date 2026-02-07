package main

import (
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// Mock of os.DirEntry for testing purposes
type mockDirEntry struct {
	name string
}

func (m *mockDirEntry) Name() string {
	return m.name
}

func (m *mockDirEntry) IsDir() bool {
	return false
}

func (m *mockDirEntry) Type() fs.FileMode {
	return 0
}

func (m *mockDirEntry) Info() (fs.FileInfo, error) {
	return nil, nil
}

// Testing ensureJPEGDirectoryExists function
func TestEnsureJPEGDirectoryExists(t *testing.T) {
	dir := os.TempDir()
	_ = ensureJPEGDirectoryExists(dir)
	jpegDir := filepath.Join(dir, "jpegs")
	if _, err := os.Stat(jpegDir); os.IsNotExist(err) {
		t.Fatalf("Directory jpegs was not created")
	}
}

// Testing getFilesInDirectory function
func TestGetFilesInDirectory(t *testing.T) {
	dir := os.TempDir()
	_, err := getFilesInDirectory(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
}

// Testing processFile function for non-HEIC files
func TestProcessFileNonHEIC(t *testing.T) {
	entry := &mockDirEntry{name: "test.txt"}
	currentDir := os.TempDir()
	jpegDir := filepath.Join(currentDir, "jpegs")
	logs := processFile(entry, currentDir, jpegDir)

	if _, exists := logs["test.txt"]; exists {
		t.Fatalf("Non-HEIC file should not be processed")
	}
}

func setupTestDir() (string, error) {
	tmpDir, err := ioutil.TempDir("", "testdir")
	if err != nil {
		return "", err
	}

	// Create a mock .heic file
	err = ioutil.WriteFile(tmpDir+"/test.heic", []byte("mock content"), 0644)
	return tmpDir, err
}

func TestProcessFiles(t *testing.T) {
	currentDir, err := setupTestDir()
	if err != nil {
		t.Fatalf("Failed to setup test directory: %v", err)
	}
	defer os.RemoveAll(currentDir)

	jpegDir := currentDir + "/jpegs"
	entries, err := os.ReadDir(currentDir)

	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	logs := processFiles(currentDir, jpegDir, entries)
	if _, ok := logs["test.heic"]; !ok {
		t.Errorf("Expected log entry for test.heic but didn't find one")
	}

}

func TestResolveInputDirectoryArg(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	os.Args = []string{"heictojpeg", "testdata/images"}

	dir, files, err := resolveInput()
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	if dir != "testdata/images" {
		t.Fatalf("expected dir testdata/images, got %s", dir)
	}
	if len(files) == 0 {
		t.Fatal("expected files from testdata/images, got none")
	}
}

func TestResolveInputFileArg(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	inputFile := "testdata/images/libheif-example.heic"
	os.Args = []string{"heictojpeg", inputFile}

	dir, files, err := resolveInput()
	if err != nil {
		t.Fatalf("resolveInput failed: %v", err)
	}
	if dir != filepath.Dir(inputFile) {
		t.Fatalf("expected dir %s, got %s", filepath.Dir(inputFile), dir)
	}
	if len(files) != 1 {
		t.Fatalf("expected one file entry, got %d", len(files))
	}
	if files[0].Name() != "libheif-example.heic" {
		t.Fatalf("expected file libheif-example.heic, got %s", files[0].Name())
	}
}

func TestOpenSourceFixturesPresent(t *testing.T) {
	fixtures := []string{
		"testdata/images/libheif-example.heic",
		"testdata/images/libheif-example.avif",
		"testdata/images/goheif-camel.heic",
	}

	for _, fixture := range fixtures {
		info, err := os.Stat(fixture)
		if err != nil {
			t.Fatalf("fixture missing (%s): %v", fixture, err)
		}
		if info.Size() == 0 {
			t.Fatalf("fixture is empty (%s)", fixture)
		}
	}
}
