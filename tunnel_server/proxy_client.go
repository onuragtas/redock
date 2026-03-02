package tunnel_server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const proxyClientTimeout = 30 * time.Second

var (
	ErrInvalidServerID = errors.New("invalid server_id")
	ErrServerNoBaseURL = errors.New("server has no base_url")
)

// ProxyResult harici tünel sunucusuna proxy isteği sonucu. Controller bunu kullanarak response yazar.
type ProxyResult struct {
	StatusCode  int
	Body        []byte
	ContentType string
}

// ProxyToExternal, userID ve serverID ile credential alır, harici sunucuya method+path+body ile istek atar.
// 401 gelirse credential silinir ve StatusCode 401 + mesaj döner. Hata durumunda StatusCode 0 ve err != nil.
func ProxyToExternal(userID, serverID uint, method, path string, body []byte) (*ProxyResult, error) {
	server, err := FindTunnelServerByID(serverID)
	if err != nil || server == nil {
		return nil, ErrInvalidServerID
	}
	baseURL := strings.TrimSpace(server.BaseURL)
	if baseURL == "" {
		return nil, ErrServerNoBaseURL
	}
	cred := CredentialByBaseURLAndUser(baseURL, userID)
	if cred == nil || cred.AccessToken == "" {
		return &ProxyResult{
			StatusCode:  http.StatusUnauthorized,
			Body:        mustJSON(map[string]interface{}{"error": true, "msg": "no credential for this tunnel server; connect via OAuth2 first"}),
			ContentType: "application/json",
		}, nil
	}
	url := strings.TrimSuffix(baseURL, "/") + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred.AccessToken)
	client := &http.Client{Timeout: proxyClientTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return &ProxyResult{
			StatusCode:  http.StatusBadGateway,
			Body:        mustJSON(map[string]interface{}{"error": true, "msg": err.Error()}),
			ContentType: "application/json",
		}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		body401, _ := io.ReadAll(resp.Body)
		if len(body401) > 0 {
			log.Printf("tunnel_server: proxy to %s returned 401: %s", url, string(body401))
		} else {
			log.Printf("tunnel_server: proxy to %s returned 401 (tunnel token expired or invalid)", url)
		}
		_ = DeleteCredentialByID(cred.ID)
		return &ProxyResult{
			StatusCode:  http.StatusUnauthorized,
			Body:        mustJSON(map[string]interface{}{"error": true, "msg": "tunnel server token expired or invalid; credential removed, please connect again via OAuth2"}),
			ContentType: "application/json",
		}, nil
	}
	respBody, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &ProxyResult{
		StatusCode:  resp.StatusCode,
		Body:        respBody,
		ContentType: contentType,
	}, nil
}

func mustJSON(m map[string]interface{}) []byte {
	b, _ := json.Marshal(m)
	return b
}
