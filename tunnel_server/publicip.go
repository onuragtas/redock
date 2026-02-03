package tunnel_server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DetectPublicIP tries external services (same as email server) to get the machine's public IP.
// Returns empty string if detection fails.
func DetectPublicIP() string {
	services := []string{
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
		"https://icanhazip.com",
	}
	for _, serviceURL := range services {
		ip, err := detectIPFromService(serviceURL)
		if err == nil && ip != "" {
			return ip
		}
	}
	return ""
}

func detectIPFromService(serviceURL string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serviceURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if len(ip) < 7 || len(ip) > 15 || !strings.Contains(ip, ".") {
		return "", fmt.Errorf("invalid IP format")
	}
	return ip, nil
}
