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

type OpenRouterClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	return &OpenRouterClient{
		apiKey:     apiKey,
		baseURL:    "https://openrouter.ai/api/v1",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// ChatCompletion calls OpenAI-compatible chat completions
func (c *OpenRouterClient) ChatCompletion(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("openrouter api key is not set")
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": 0.7,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "http://localhost:5173")
	req.Header.Set("X-Title", "VideoFlow Go")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openrouter error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", fmt.Errorf("no choices returned by openrouter")
	}

	return res.Choices[0].Message.Content, nil
}

// StructuredJSON requests JSON format from the LLM and unmarshals into target
func (c *OpenRouterClient) StructuredJSON(ctx context.Context, model, systemPrompt, userPrompt string, target interface{}) error {
	fullSystem := systemPrompt + "\n\nCRITICAL: Respond ONLY with valid, raw JSON. Do not include markdown codeblocks or explanation."
	content, err := c.ChatCompletion(ctx, model, fullSystem, userPrompt)
	if err != nil {
		return err
	}

	// Strip potential markdown backticks ```json ... ```
	cleaned := bytes.TrimSpace([]byte(content))
	if bytes.HasPrefix(cleaned, []byte("```json")) {
		cleaned = bytes.TrimPrefix(cleaned, []byte("```json"))
	}
	if bytes.HasPrefix(cleaned, []byte("```")) {
		cleaned = bytes.TrimPrefix(cleaned, []byte("```"))
	}
	if bytes.HasSuffix(cleaned, []byte("```")) {
		cleaned = bytes.TrimSuffix(cleaned, []byte("```"))
	}
	cleaned = bytes.TrimSpace(cleaned)

	return json.Unmarshal(cleaned, target)
}

// CreateVideo submits a video generation job to OpenRouter
func (c *OpenRouterClient) CreateVideo(ctx context.Context, payload map[string]interface{}) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("openrouter api key is not set")
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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("video generation failed (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", fmt.Errorf("no job id returned: %s", string(bodyBytes))
	}
	return res.ID, nil
}

// GetVideo polls status of a video job
func (c *OpenRouterClient) GetVideo(ctx context.Context, jobID string) (status string, outputURLs []string, err error) {
	if c.apiKey == "" {
		return "", nil, fmt.Errorf("openrouter api key is not set")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/videos/"+jobID, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}

	var res struct {
		Status string      `json:"status"`
		Output interface{} `json:"output"`
		Error  *string     `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", nil, err
	}

	if res.Error != nil && *res.Error != "" {
		return "failed", nil, fmt.Errorf("video error: %s", *res.Error)
	}

	var urls []string
	switch v := res.Output.(type) {
	case string:
		urls = append(urls, v)
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				urls = append(urls, s)
			}
		}
	}

	return res.Status, urls, nil
}
