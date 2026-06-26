package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// PasteListItem represents a single paste entry in the list view.
type PasteListItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Language  string    `json:"language"`
	Views     int       `json:"views"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// listResponse is the API shape for /api/v1/pastes.
type listResponse struct {
	Pastes     []PasteListItem `json:"pastes"`
	Pagination struct {
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

// apiClient is a lightweight HTTP client used by the TUI API helpers.
type apiClient struct {
	server string
	lang   string
}

// newAPIClient creates an apiClient with the given server base URL and language.
func newAPIClient(server, lang string) *apiClient {
	return &apiClient{server: server, lang: lang}
}

// get performs a GET request, setting the standard TUI headers.
func (a *apiClient) get(path string) (*http.Response, error) {
	hc := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, a.server+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pastebin-cli/"+clientVersion)
	req.Header.Set("Accept", "application/json")
	if a.lang != "" {
		req.Header.Set("Accept-Language", a.lang)
	}
	return hc.Do(req)
}

// fetchPastes retrieves a page of public pastes from the server.
func fetchPastes(server, lang string, page, limit int) ([]PasteListItem, error) {
	a := newAPIClient(server, lang)
	path := fmt.Sprintf("/api/v1/pastes?page=%d&limit=%d", page, limit)
	resp, err := a.get(path)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var lr listResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return lr.Pastes, nil
}

// fetchPasteRaw retrieves the raw text content of a single paste.
func fetchPasteRaw(server, lang, id string) (string, error) {
	a := newAPIClient(server, lang)
	resp, err := a.get("/raw/" + id)
	if err != nil {
		return "", fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("paste %q not found or has expired", id)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	return string(buf), nil
}

// deletePaste sends a DELETE request to remove a paste using its delete token.
func deletePaste(server, lang, id, token string) error {
	hc := &http.Client{Timeout: 15 * time.Second}
	path := server + "/api/v1/pastes/" + id + "?token=" + url.QueryEscape(token)
	req, err := http.NewRequest(http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pastebin-cli/"+clientVersion)
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("paste %q not found or invalid token", id)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// clientVersion is set at build time; the TUI uses it in User-Agent.
// It mirrors the Version var in main.go which is injected via ldflags.
// We use a package-level var so tests can override it without forking the binary.
var clientVersion = "dev"
