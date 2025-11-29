package api_gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TelemetryExporter handles exporting request/response data to external systems
type TelemetryExporter struct {
	config     *ObservabilityConfig
	buffer     []RequestLog
	mu         sync.Mutex
	stopChan   chan struct{}
	running    bool
	httpClient *http.Client
}

var (
	telemetryExporter     *TelemetryExporter
	telemetryExporterOnce sync.Once
)

// GetTelemetryExporter returns the singleton telemetry exporter
func GetTelemetryExporter() *TelemetryExporter {
	telemetryExporterOnce.Do(func() {
		telemetryExporter = &TelemetryExporter{
			buffer:   make([]RequestLog, 0),
			stopChan: make(chan struct{}),
			httpClient: &http.Client{
				Timeout: 10 * time.Second,
			},
		}
	})
	return telemetryExporter
}

// Configure updates the telemetry exporter configuration
func (e *TelemetryExporter) Configure(config *ObservabilityConfig) {
	e.mu.Lock()
	e.config = config
	e.mu.Unlock()
}

// Start starts the telemetry exporter
func (e *TelemetryExporter) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stopChan = make(chan struct{})
	e.mu.Unlock()

	go e.flushLoop()
	log.Println("API Gateway: Telemetry exporter started")
}

// Stop stops the telemetry exporter
func (e *TelemetryExporter) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stopChan)
	e.mu.Unlock()

	// Flush remaining data
	e.flush()
	log.Println("API Gateway: Telemetry exporter stopped")
}

// IsRunning returns whether the exporter is running
func (e *TelemetryExporter) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Record records a request log entry
func (e *TelemetryExporter) Record(logEntry RequestLog) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.config == nil || !e.config.Enabled {
		return
	}

	e.buffer = append(e.buffer, logEntry)

	// Check if we should flush based on batch size
	batchSize := e.config.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	if len(e.buffer) >= batchSize {
		// Copy data and clear buffer while holding lock to avoid race conditions
		data := make([]RequestLog, len(e.buffer))
		copy(data, e.buffer)
		e.buffer = e.buffer[:0]
		config := e.config

		// Release lock before sending to avoid blocking
		e.mu.Unlock()
		e.sendToEndpoints(data, config)
		e.mu.Lock()
	}
}

// sendToEndpoints sends data to all configured endpoints
func (e *TelemetryExporter) sendToEndpoints(data []RequestLog, config *ObservabilityConfig) {
	if config == nil {
		return
	}

	if lokiCfg := e.resolveLokiConfig(config); lokiCfg != nil {
		go e.sendToLoki(lokiCfg, data)
	}

	if influxCfg := e.resolveInfluxConfig(config); influxCfg != nil {
		go e.sendToInfluxDB(influxCfg, data)
	}

	if graylogCfg := e.resolveGraylogConfig(config); graylogCfg != nil {
		go e.sendToGraylog(graylogCfg, data)
	}

	if config.OTLPEnabled && config.OTLPEndpoint != "" {
		go e.sendToOTLP(data, config)
	}

	if config.ClickHouseEnabled && config.ClickHouseEndpoint != "" {
		go e.sendToClickHouse(data, config)
	}
}

func (e *TelemetryExporter) resolveLokiConfig(config *ObservabilityConfig) *LokiDatasourceConfig {
	if config == nil {
		return nil
	}
	if config.LokiEnabled && config.Loki != nil && config.Loki.URL != "" {
		return config.Loki
	}
	return nil
}

func (e *TelemetryExporter) resolveInfluxConfig(config *ObservabilityConfig) *InfluxDBDatasourceConfig {
	if config == nil {
		return nil
	}
	if config.InfluxEnabled && config.InfluxDB != nil && config.InfluxDB.URL != "" {
		return config.InfluxDB
	}
	return nil
}

func (e *TelemetryExporter) resolveGraylogConfig(config *ObservabilityConfig) *GraylogConfig {
	if config == nil {
		return nil
	}
	if config.GraylogEnabled && config.Graylog != nil && config.Graylog.Endpoint != "" {
		return config.Graylog
	}
	return nil
}

// flushLoop periodically flushes the buffer
func (e *TelemetryExporter) flushLoop() {
	e.mu.Lock()
	interval := 30 * time.Second
	if e.config != nil && e.config.FlushInterval > 0 {
		interval = time.Duration(e.config.FlushInterval) * time.Second
	}
	e.mu.Unlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.flush()
		}
	}
}

// flush sends the buffered data to configured endpoints
func (e *TelemetryExporter) flush() {
	e.mu.Lock()
	if len(e.buffer) == 0 || e.config == nil {
		e.mu.Unlock()
		return
	}

	data := make([]RequestLog, len(e.buffer))
	copy(data, e.buffer)
	e.buffer = e.buffer[:0]
	config := e.config
	e.mu.Unlock()

	// Send to configured endpoints using the helper
	e.sendToEndpoints(data, config)
}

// sendToLoki sends data to a Loki datasource
func (e *TelemetryExporter) sendToLoki(lokiCfg *LokiDatasourceConfig, data []RequestLog) {
	endpoint := strings.TrimSpace(lokiCfg.URL)
	if endpoint == "" {
		log.Printf("API Gateway Telemetry: Loki endpoint missing")
		return
	}

	// Convert to Loki format
	streams := make([]map[string]interface{}, 0)

	for _, entry := range data {
		values := [][]interface{}{
			{
				fmt.Sprintf("%d", entry.Timestamp.UnixNano()),
				fmt.Sprintf("method=%s path=%s status=%d duration=%dms service=%s",
					entry.Method, entry.Path, entry.StatusCode, entry.Duration, entry.ServiceID),
			},
		}

		labels := map[string]string{
			"job":        "api_gateway",
			"service_id": entry.ServiceID,
			"route_id":   entry.RouteID,
		}
		for k, v := range lokiCfg.Labels {
			labels[k] = v
		}
		stream := map[string]interface{}{
			"stream": labels,
			"values": values,
		}
		streams = append(streams, stream)
	}

	payload := map[string]interface{}{
		"streams": streams,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to marshal Loki data: %v", err)
		return
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to create Loki request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if lokiCfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+lokiCfg.APIKey)
	}
	if lokiCfg.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", lokiCfg.TenantID)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to send to Loki: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("API Gateway Telemetry: Loki returned status %d", resp.StatusCode)
	}
}

func (e *TelemetryExporter) sendToInfluxDB(influxCfg *InfluxDBDatasourceConfig, data []RequestLog) {
	if influxCfg.URL == "" || influxCfg.Org == "" || influxCfg.Bucket == "" || influxCfg.Token == "" {
		log.Printf("API Gateway Telemetry: InfluxDB config missing url/org/bucket/token")
		return
	}

	var builder strings.Builder
	for _, entry := range data {
		builder.WriteString("api_gateway")
		builder.WriteString(",route_id=")
		builder.WriteString(escapeInfluxTag(entry.RouteID))
		builder.WriteString(",service_id=")
		builder.WriteString(escapeInfluxTag(entry.ServiceID))

		builder.WriteString(" status_code=")
		builder.WriteString(fmt.Sprintf("%di", entry.StatusCode))
		builder.WriteString(",duration_ms=")
		builder.WriteString(fmt.Sprintf("%di", entry.Duration))
		builder.WriteString(",bytes_sent=")
		builder.WriteString(fmt.Sprintf("%di", entry.BytesSent))
		builder.WriteString(",bytes_received=")
		builder.WriteString(fmt.Sprintf("%di", entry.BytesReceived))
		builder.WriteString(",success=")
		if entry.StatusCode < 400 {
			builder.WriteString("true")
		} else {
			builder.WriteString("false")
		}
		if entry.Error != "" {
			builder.WriteString(",error_message=")
			builder.WriteString(escapeInfluxStringField(entry.Error))
		}
		builder.WriteString(",method=")
		builder.WriteString(escapeInfluxStringField(entry.Method))
		builder.WriteString(",path=")
		builder.WriteString(escapeInfluxStringField(entry.Path))
		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%d", entry.Timestamp.UnixNano()))
		builder.WriteString("\n")
	}

	query := fmt.Sprintf("%s/api/v2/write?org=%s&bucket=%s&precision=ns",
		strings.TrimRight(influxCfg.URL, "/"), url.QueryEscape(influxCfg.Org), url.QueryEscape(influxCfg.Bucket))
	req, err := http.NewRequest("POST", query, strings.NewReader(builder.String()))
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to create InfluxDB request: %v", err)
		return
	}
	req.Header.Set("Authorization", "Token "+influxCfg.Token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to send to InfluxDB: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("API Gateway Telemetry: InfluxDB returned status %d", resp.StatusCode)
	}
}

func (e *TelemetryExporter) sendToGraylog(graylogCfg *GraylogConfig, data []RequestLog) {
	endpoint := strings.TrimSpace(graylogCfg.Endpoint)
	if endpoint == "" {
		log.Printf("API Gateway Telemetry: Graylog endpoint missing")
		return
	}

	headerName := strings.TrimSpace(graylogCfg.APIKeyHeader)
	if headerName == "" {
		headerName = "Authorization"
	}

	for _, entry := range data {
		payload := map[string]interface{}{
			"version":       "1.1",
			"host":          entry.Host,
			"short_message": fmt.Sprintf("%s %s -> %d", entry.Method, entry.Path, entry.StatusCode),
			"timestamp":     float64(entry.Timestamp.UnixNano()) / float64(time.Second),
			"level":         graylogLevelForStatus(entry.StatusCode),
			"_route_id":     entry.RouteID,
			"_service_id":   entry.ServiceID,
			"_duration_ms":  entry.Duration,
			"_bytes_sent":   entry.BytesSent,
			"_bytes_recv":   entry.BytesReceived,
			"_client_ip":    entry.RemoteAddr,
			"_user_agent":   entry.UserAgent,
		}
		if entry.Error != "" {
			payload["_error"] = entry.Error
		}
		if graylogCfg.StreamID != "" {
			payload["_stream_id"] = graylogCfg.StreamID
		}
		for key, value := range graylogCfg.ExtraFields {
			if key == "" {
				continue
			}
			payload["_"+key] = value
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			log.Printf("API Gateway Telemetry: Failed to marshal Graylog data: %v", err)
			continue
		}

		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("API Gateway Telemetry: Failed to create Graylog request: %v", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if graylogCfg.APIKey != "" {
			req.Header.Set(headerName, graylogCfg.APIKey)
		}

		resp, err := e.httpClient.Do(req)
		if err != nil {
			log.Printf("API Gateway Telemetry: Failed to send to Graylog: %v", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			log.Printf("API Gateway Telemetry: Graylog returned status %d", resp.StatusCode)
		}
	}
}

func graylogLevelForStatus(status int) int {
	switch {
	case status >= 500:
		return 3 // Error
	case status >= 400:
		return 4 // Warning
	default:
		return 6 // Informational
	}
}

func escapeInfluxTag(value string) string {
	escaped := strings.NewReplacer(",", "\\,", " ", "\\ ", "=", "\\=").Replace(value)
	return escaped
}

func escapeInfluxStringField(value string) string {
	escaped := strings.ReplaceAll(value, "\"", "\\\"")
	return fmt.Sprintf("\"%s\"", escaped)
}

// sendToOTLP sends data to OpenTelemetry collector
func (e *TelemetryExporter) sendToOTLP(data []RequestLog, config *ObservabilityConfig) {
	// Convert to OTLP format
	spans := make([]map[string]interface{}, 0)

	for _, entry := range data {
		span := map[string]interface{}{
			"traceId":           fmt.Sprintf("%x", entry.Timestamp.UnixNano()),
			"spanId":            fmt.Sprintf("%x", time.Now().UnixNano()),
			"name":              fmt.Sprintf("%s %s", entry.Method, entry.Path),
			"kind":              2, // SPAN_KIND_SERVER
			"startTimeUnixNano": entry.Timestamp.UnixNano(),
			"endTimeUnixNano":   entry.Timestamp.Add(time.Duration(entry.Duration) * time.Millisecond).UnixNano(),
			"attributes": []map[string]interface{}{
				{"key": "http.method", "value": map[string]string{"stringValue": entry.Method}},
				{"key": "http.url", "value": map[string]string{"stringValue": entry.Path}},
				{"key": "http.status_code", "value": map[string]int{"intValue": entry.StatusCode}},
				{"key": "http.host", "value": map[string]string{"stringValue": entry.Host}},
				{"key": "service.id", "value": map[string]string{"stringValue": entry.ServiceID}},
				{"key": "route.id", "value": map[string]string{"stringValue": entry.RouteID}},
			},
			"status": map[string]interface{}{
				"code": func() int {
					if entry.StatusCode >= 400 {
						return 2 // ERROR
					}
					return 1 // OK
				}(),
			},
		}
		spans = append(spans, span)
	}

	payload := map[string]interface{}{
		"resourceSpans": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{"key": "service.name", "value": map[string]string{"stringValue": "api_gateway"}},
					},
				},
				"scopeSpans": []map[string]interface{}{
					{
						"scope": map[string]interface{}{
							"name": "api_gateway",
						},
						"spans": spans,
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to marshal OTLP data: %v", err)
		return
	}

	req, err := http.NewRequest("POST", config.OTLPEndpoint+"/v1/traces", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to create OTLP request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range config.OTLPHeaders {
		req.Header.Set(key, value)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to send to OTLP: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("API Gateway Telemetry: OTLP returned status %d", resp.StatusCode)
	}
}

// sendToClickHouse sends data to ClickHouse
func (e *TelemetryExporter) sendToClickHouse(data []RequestLog, config *ObservabilityConfig) {
	// Build INSERT query
	database := config.ClickHouseDatabase
	if database == "" {
		database = "default"
	}
	table := config.ClickHouseTable
	if table == "" {
		table = "api_gateway_logs"
	}

	// Convert to ClickHouse JSON format
	rows := make([]map[string]interface{}, 0, len(data))
	for _, entry := range data {
		row := map[string]interface{}{
			"timestamp":      entry.Timestamp.Format("2006-01-02 15:04:05"),
			"method":         entry.Method,
			"path":           entry.Path,
			"host":           entry.Host,
			"remote_addr":    entry.RemoteAddr,
			"route_id":       entry.RouteID,
			"service_id":     entry.ServiceID,
			"status_code":    entry.StatusCode,
			"duration_ms":    entry.Duration,
			"bytes_sent":     entry.BytesSent,
			"bytes_received": entry.BytesReceived,
			"user_agent":     entry.UserAgent,
			"error":          entry.Error,
		}
		rows = append(rows, row)
	}

	jsonData, err := json.Marshal(rows)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to marshal ClickHouse data: %v", err)
		return
	}

	url := fmt.Sprintf("%s/?database=%s&query=INSERT INTO %s FORMAT JSONEachRow",
		config.ClickHouseEndpoint, database, table)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to create ClickHouse request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if config.ClickHouseUsername != "" {
		req.SetBasicAuth(config.ClickHouseUsername, config.ClickHousePassword)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		log.Printf("API Gateway Telemetry: Failed to send to ClickHouse: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("API Gateway Telemetry: ClickHouse returned status %d", resp.StatusCode)
	}
}

// GetStatus returns the current telemetry exporter status
func (e *TelemetryExporter) GetStatus() map[string]interface{} {
	e.mu.Lock()
	defer e.mu.Unlock()

	status := map[string]interface{}{
		"running":     e.running,
		"buffer_size": len(e.buffer),
	}

	if e.config != nil {
		status["enabled"] = e.config.Enabled
		status["loki_enabled"] = e.config.LokiEnabled
		status["influx_enabled"] = e.config.InfluxEnabled
		status["graylog_enabled"] = e.config.GraylogEnabled
		status["otlp_enabled"] = e.config.OTLPEnabled
		status["clickhouse_enabled"] = e.config.ClickHouseEnabled
	}

	return status
}
