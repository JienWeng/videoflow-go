package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AtlasCloudClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAtlasCloudClient(apiKey string) *AtlasCloudClient {
	return &AtlasCloudClient{
		apiKey:     apiKey,
		baseURL:    "https://api.atlascloud.ai/v1",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *AtlasCloudClient) SubmitVideo(ctx context.Context, payload map[string]interface{}) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("atlascloud api key is not set")
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/videos", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}
	return res.ID, nil
}

func (c *AtlasCloudClient) PollVideo(ctx context.Context, jobID string) (status string, outputURL string, err error) {
	if c.apiKey == "" {
		return "", "", fmt.Errorf("atlascloud api key is not set")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/videos/"+jobID, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	var res struct {
		Status string `json:"status"`
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", "", err
	}

	return res.Status, res.Output, nil
}
