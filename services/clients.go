package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var (
	ErrUserNotFound    = errors.New("korisnik nije pronađen")
	ErrServiceCallFail = errors.New("greška pri sinhronoj komunikaciji sa servisom")
)

type QuotaResponse struct {
	UserID string `json:"user_id"`
	Quota  int64  `json:"quota"`
}

type ExistsResponse struct {
	UserID string `json:"user_id"`
	Exists bool   `json:"exists"`
}

type UsedStorageResponse struct {
	UserID    string `json:"user_id"`
	UsedBytes int64  `json:"used_bytes"`
}

type UsersClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewUsersClient(baseURL string) *UsersClient {
	return &UsersClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *UsersClient) GetUserQuota(ctx context.Context, userID string) (int64, error) {
	u := fmt.Sprintf("%s/api/internal/users/%s/quota", c.baseURL, url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrServiceCallFail, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrServiceCallFail, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return 0, ErrUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: neočekivan status %d", ErrServiceCallFail, resp.StatusCode)
	}

	var payload QuotaResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("%w: greška pri dekodiranju odgovora: %v", ErrServiceCallFail, err)
	}

	return payload.Quota, nil
}

func (c *UsersClient) UserExists(ctx context.Context, userID string) (bool, error) {
	u := fmt.Sprintf("%s/api/internal/users/%s/exists", c.baseURL, url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrServiceCallFail, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrServiceCallFail, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, ErrUserNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%w: neočekivan status %d", ErrServiceCallFail, resp.StatusCode)
	}

	var payload ExistsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("%w: greška pri dekodiranju odgovora: %v", ErrServiceCallFail, err)
	}

	return payload.Exists, nil
}

type FilesClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewFilesClient(baseURL string) *FilesClient {
	return &FilesClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *FilesClient) GetUsedStorage(ctx context.Context, userID string) (int64, error) {
	u := fmt.Sprintf("%s/api/internal/files/used?user_id=%s", c.baseURL, url.QueryEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrServiceCallFail, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrServiceCallFail, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: neočekivan status %d", ErrServiceCallFail, resp.StatusCode)
	}

	var payload UsedStorageResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("%w: greška pri dekodiranju odgovora: %v", ErrServiceCallFail, err)
	}

	return payload.UsedBytes, nil
}
