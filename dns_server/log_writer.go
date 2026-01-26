package dns_server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	dockermanager "redock/docker-manager"
	"time"
)

// DNSLogWriter handles DNS query logging to JSONL files
type DNSLogWriter struct {
	dockerManager *dockermanager.DockerEnvironmentManager
	logChannel    chan DNSQueryLog
	ctx           context.Context
	cancel        context.CancelFunc
	currentDate   string
	currentFile   *os.File
}

// NewDNSLogWriter creates a new log writer
func NewDNSLogWriter(dockerManager *dockermanager.DockerEnvironmentManager) (*DNSLogWriter, error) {
	// Ensure data directory exists
	dataDir := filepath.Join(dockerManager.GetWorkDir(), "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	writer := &DNSLogWriter{
		dockerManager: dockerManager,
		logChannel:    make(chan DNSQueryLog, 1000),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start writer goroutine
	go writer.writerLoop()

	// Start daily cleanup goroutine
	go writer.cleanupLoop()

	return writer, nil
}

// LogQuery sends a query to the log channel (non-blocking)
func (w *DNSLogWriter) LogQuery(log DNSQueryLog) {
	select {
	case w.logChannel <- log:
		// Log sent successfully
	default:
		// Channel full, drop log (better than blocking DNS queries)
	}
}

// writerLoop is the single writer goroutine
func (w *DNSLogWriter) writerLoop() {
	buffer := make([]DNSQueryLog, 0, 100)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			// Final flush on shutdown
			w.flushBuffer(buffer)
			if w.currentFile != nil {
				w.currentFile.Close()
			}
			return

		case log := <-w.logChannel:
			buffer = append(buffer, log)
			// Flush when buffer full
			if len(buffer) >= 50 {
				w.flushBuffer(buffer)
				buffer = buffer[:0]
			}

		case <-ticker.C:
			// Time-based flush (every 10 seconds)
			if len(buffer) > 0 {
				w.flushBuffer(buffer)
				buffer = buffer[:0]
			}
			// Rotate file if date changed
			w.checkDateRotation()
		}
	}
}

// flushBuffer writes buffered logs to JSONL file
func (w *DNSLogWriter) flushBuffer(logs []DNSQueryLog) {
	if len(logs) == 0 {
		return
	}

	// Open/rotate file if needed
	if err := w.ensureFile(); err != nil {
		log.Printf("⚠️  Failed to open log file: %v", err)
		return
	}

	// Write each log as JSON line
	for i := range logs {
		data, err := json.Marshal(&logs[i])
		if err != nil {
			continue
		}

		// Append line
		if _, err := w.currentFile.Write(append(data, '\n')); err != nil {
			log.Printf("⚠️  Failed to write log: %v", err)
			return
		}
	}

	// Sync to disk
	w.currentFile.Sync()
}

// ensureFile ensures log file for current date is open
func (w *DNSLogWriter) ensureFile() error {
	date := time.Now().Format("2006-01-02")

	if w.currentDate == date && w.currentFile != nil {
		return nil
	}

	// Close old file
	if w.currentFile != nil {
		w.currentFile.Close()
	}

	// Open new file for current date
	filename := fmt.Sprintf("dns_logs_%s.jsonl", date)
	dataDir := filepath.Join(w.dockerManager.GetWorkDir(), "data")
	path := filepath.Join(dataDir, filename)

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	w.currentFile = file
	w.currentDate = date
	return nil
}

// checkDateRotation checks if date changed and rotates file
func (w *DNSLogWriter) checkDateRotation() {
	date := time.Now().Format("2006-01-02")
	if w.currentDate != date {
		if w.currentFile != nil {
			w.currentFile.Close()
			w.currentFile = nil
		}
		w.currentDate = ""
	}
}

// cleanupLoop removes old log files
func (w *DNSLogWriter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.cleanup(7) // Keep last 7 days
		}
	}
}

// cleanup removes JSONL files older than retentionDays
func (w *DNSLogWriter) cleanup(retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	dataDir := filepath.Join(w.dockerManager.GetWorkDir(), "data")
	pattern := filepath.Join(dataDir, "dns_logs_*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
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
}

// Stop stops the log writer
func (w *DNSLogWriter) Stop() {
	w.cancel()
}

// ReadLogs reads logs from JSONL files with time filter (streaming)
func (w *DNSLogWriter) ReadLogs(since time.Time, callback func(DNSQueryLog) error) error {
	dataDir := filepath.Join(w.dockerManager.GetWorkDir(), "data")
	pattern := filepath.Join(dataDir, "dns_logs_*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, filename := range files {
		file, err := os.Open(filename)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			var log DNSQueryLog
			if err := json.Unmarshal(scanner.Bytes(), &log); err != nil {
				continue
			}

			if log.CreatedAt.After(since) {
				if err := callback(log); err != nil {
					file.Close()
					return err
				}
			}
		}

		file.Close()
	}

	return nil
}

// GetLogCount returns total log count since time (streaming count)
func (w *DNSLogWriter) GetLogCount(since time.Time, blockedOnly bool) (int64, error) {
	var count int64

	err := w.ReadLogs(since, func(log DNSQueryLog) error {
		if blockedOnly && !log.Blocked {
			return nil
		}
		count++
		return nil
	})

	return count, err
}
