package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type BytePlusClient struct{ mediaHTTP }

func NewBytePlusClientAt(key, base string) *BytePlusClient {
	return &BytePlusClient{newMediaHTTP(key, base, "BytePlus ModelArk", "Bearer ")}
}
func (c *BytePlusClient) Models(ctx context.Context, kind string) ([]Model, error) {
	return mediaCatalog("byteplus", kind), nil
}
func (c *BytePlusClient) Model(ctx context.Context, id, kind string) (*Model, error) {
	return catalogModel("byteplus", id, kind)
}
func (c *BytePlusClient) ValidateImage(ctx context.Context, p map[string]any) error {
	return fmt.Errorf("BytePlus connection supports video only")
}
func (c *BytePlusClient) GenerateImages(ctx context.Context, p map[string]any) ([]GeneratedImage, error) {
	return nil, c.ValidateImage(ctx, p)
}
func (c *BytePlusClient) ValidateVideo(ctx context.Context, p map[string]any) error {
	return validateMediaVideo("byteplus", ctx, p)
}
func (c *BytePlusClient) CreateVideo(ctx context.Context, p map[string]any) (string, error) {
	if err := c.ValidateVideo(ctx, p); err != nil {
		return "", err
	}
	content := []map[string]any{{"type": "text", "text": p["prompt"]}}
	frames, _ := videoFrames(p)
	for _, f := range frames {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": f.ImageURL.URL}, "role": f.FrameType})
	}
	out := map[string]any{"model": p["model"], "content": content}
	for _, k := range []string{"duration", "resolution", "generate_audio", "seed"} {
		if v, ok := p[k]; ok {
			out[k] = v
		}
	}
	if a, ok := p["aspect_ratio"]; ok {
		out["ratio"] = a
	}
	b, err := c.request(ctx, "POST", "/contents/generations/tasks", out)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(b, &result) != nil || result.ID == "" {
		return "", fmt.Errorf("BytePlus returned no task ID")
	}
	return result.ID, nil
}
func (c *BytePlusClient) GetVideo(ctx context.Context, id string) (string, []string, error) {
	if id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", nil, fmt.Errorf("invalid BytePlus task ID")
	}
	b, err := c.request(ctx, "GET", "/contents/generations/tasks/"+url.PathEscape(id), nil)
	if err != nil {
		return "", nil, err
	}
	var res struct {
		Status  string `json:"status"`
		Content struct {
			URL string `json:"video_url"`
		} `json:"content"`
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(b, &res) != nil {
		return "", nil, fmt.Errorf("invalid BytePlus task response")
	}
	switch res.Status {
	case "queued":
		return "pending", nil, nil
	case "running":
		return "running", nil, nil
	case "succeeded":
		if res.Content.URL == "" {
			return "failed", nil, fmt.Errorf("BytePlus result contains no video")
		}
		return "completed", []string{res.Content.URL}, nil
	case "failed", "expired", "cancelled", "canceled":
		message := string(res.Error)
		if c.key != "" {
			message = strings.ReplaceAll(message, c.key, "[redacted]")
		}
		return "failed", nil, fmt.Errorf("BytePlus task %s: %s", res.Status, message)
	default:
		return "", nil, fmt.Errorf("unknown BytePlus task status %q", res.Status)
	}
}
func (c *BytePlusClient) VideoContent(ctx context.Context, id string, index int) ([]byte, error) {
	return videoBytes(ctx, c, id, index, c.base)
}
