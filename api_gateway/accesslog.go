package api_gateway

import (
	"bytes"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// accessLogWriter writes access log lines off the request path. A synchronous
// log.Printf per request costs a write syscall (and, as a service, journald
// work) under the log package's global lock, which serializes every request.
// Here requests only marshal their line and hand it to a bounded queue; one
// goroutine writes queued lines in batches. When the queue is full the line
// is dropped and counted — the proxy never waits on logging.
type accessLogWriter struct {
	once    sync.Once
	queue   chan []byte
	dropped atomic.Int64
}

const (
	accessLogQueueSize = 8192
	accessLogBatchMax  = 256
)

var accessLog = &accessLogWriter{}

func (w *accessLogWriter) start() {
	w.queue = make(chan []byte, accessLogQueueSize)
	go w.run()
}

// enqueue queues one already-formatted line (without trailing newline).
func (w *accessLogWriter) enqueue(line []byte) {
	w.once.Do(w.start)
	select {
	case w.queue <- line:
	default:
		w.dropped.Add(1)
	}
}

func (w *accessLogWriter) run() {
	var buf bytes.Buffer
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case line := <-w.queue:
			buf.Reset()
			w.appendLine(&buf, line)
			// Drain whatever else is already queued into the same write.
		drain:
			for i := 1; i < accessLogBatchMax; i++ {
				select {
				case more := <-w.queue:
					w.appendLine(&buf, more)
				default:
					break drain
				}
			}
			_, _ = log.Writer().Write(buf.Bytes())
		case <-ticker.C:
			if n := w.dropped.Swap(0); n > 0 {
				log.Printf("API Gateway: access log queue full, %d lines dropped in the last minute", n)
			}
		}
	}
}

// appendLine formats a line like log.Printf with the standard flags would.
func (w *accessLogWriter) appendLine(buf *bytes.Buffer, line []byte) {
	buf.WriteString(time.Now().Format("2006/01/02 15:04:05 "))
	buf.WriteString("API Gateway Access: ")
	buf.Write(line)
	buf.WriteByte('\n')
}

// writeAccessLog queues the console access-log line for entry. Request and
// response bodies are left out: the console log ends up in journald, and
// bodies carry passwords and tokens. Telemetry exporters still receive them.
func writeAccessLog(entry RequestLog) {
	entry.RequestBody = ""
	entry.ResponseBody = ""
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	accessLog.enqueue(line)
}
