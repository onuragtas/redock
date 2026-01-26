package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// JSONStorage provides atomic JSON file operations
type JSONStorage struct {
	baseDir string
	mutex   sync.RWMutex
}

// NewJSONStorage creates a new JSON storage
func NewJSONStorage(baseDir string) (*JSONStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &JSONStorage{baseDir: baseDir}, nil
}

// GetBaseDir returns the base directory
func (s *JSONStorage) GetBaseDir() string {
	return s.baseDir
}

// Load loads JSON file into target struct
func (s *JSONStorage) Load(filename string, target interface{}) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	path := filepath.Join(s.baseDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, not an error
		}
		return fmt.Errorf("failed to read file: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}

// Save atomically saves data to JSON file
// Uses temp file + rename for atomic operation (crash-safe)
func (s *JSONStorage) Save(filename string, data interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	path := filepath.Join(s.baseDir, filename)
	tmpPath := path + ".tmp"

	// Marshal with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temp file
	if err := os.WriteFile(tmpPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename (POSIX guarantees atomicity)
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Cleanup temp file on error
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// AppendJSONL appends a JSON line to JSONL file
func (s *JSONStorage) AppendJSONL(filename string, data interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	path := filepath.Join(s.baseDir, filename)

	// Marshal to single line
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Append line
	if _, err := file.Write(append(jsonData, '\n')); err != nil {
		return fmt.Errorf("failed to write line: %w", err)
	}

	return nil
}

// ReadJSONL reads JSONL file with optional time filter (streaming)
func (s *JSONStorage) ReadJSONL(filename string, since time.Time, callback func([]byte) error) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	path := filepath.Join(s.baseDir, filename)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, not an error
		}
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines (default 64KB)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024) // Max 1MB per line

	for scanner.Scan() {
		if err := callback(scanner.Bytes()); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	return nil
}

// RotateJSONL rotates JSONL file by date
func (s *JSONStorage) RotateJSONL(baseFilename string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Create dated filename: logs_2026-01-25.jsonl
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s_%s.jsonl", baseFilename, date)
	path := filepath.Join(s.baseDir, filename)

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	return filename, nil
}

// CleanupOldJSONL removes JSONL files older than retentionDays
func (s *JSONStorage) CleanupOldJSONL(pattern string, retentionDays int) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	files, err := filepath.Glob(filepath.Join(s.baseDir, pattern))
	if err != nil {
		return err
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(file)
		}
	}

	return nil
}
