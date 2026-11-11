package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FalClient struct {
	mediaHTTP
	model        string
	pollInterval time.Duration
}

func NewFalClientAt(key, base, model string) *FalClient {
	return &FalClient{mediaHTTP: newMediaHTTP(key, base, "fal.ai", "Key "), model: model, pollInterval: time.Second}
}
func (c *FalClient) Models(ctx context.Context, kind string) ([]Model, error) {
	if kind == "all" {
		return append(mediaCatalog("fal", "image"), mediaCatalog("fal", "video")...), nil
	}
	return mediaCatalog("fal", kind), nil
}
func (c *FalClient) Model(ctx context.Context, id, kind string) (*Model, error) {
	return catalogModel("fal", id, kind)
}
func (c *FalClient) ValidateImage(ctx context.Context, p map[string]any) error {
	m, err := c.Model(ctx, stringValue(p["model"]), "image")
	if err != nil {
		return err
	}
	if err = m.validateImage(p); err != nil {
		return err
	}
	refs, err := imageReferences(p)
	if err != nil {
		return err
	}
	if strings.HasSuffix(m.ID, "/edit") && len(refs) == 0 {
		return fmt.Errorf("fal edit endpoint requires reference images")
	}
	return nil
}
func (c *FalClient) ValidateVideo(ctx context.Context, p map[string]any) error {
	return validateMediaVideo("fal", ctx, p)
}
func stringValue(v any) string { s, _ := v.(string); return s }
func (c *FalClient) submit(ctx context.Context, model string, p map[string]any) (string, error) {
	b, err := c.request(ctx, "POST", "/"+model, p)
	if err != nil {
		return "", err
	}
	var result struct {
		ID string `json:"request_id"`
	}
	if json.Unmarshal(b, &result) != nil || result.ID == "" {
		return "", fmt.Errorf("fal returned no request ID")
	}
	return result.ID, nil
}
func falRequestPath(model, id string) (string, error) {
	parts := strings.Split(model, "/")
	if len(parts) < 2 || id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", fmt.Errorf("invalid fal model/request ID")
	}
	return "/" + strings.Join(parts[:2], "/") + "/requests/" + url.PathEscape(id), nil
}
func (c *FalClient) result(ctx context.Context, model, id string) (string, []byte, error) {
	path, err := falRequestPath(model, id)
	if err != nil {
		return "", nil, err
	}
	b, err := c.request(ctx, "GET", path+"/status", nil)
	if err != nil {
		return "", nil, err
	}
	var status struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(b, &status) != nil {
		return "", nil, fmt.Errorf("invalid fal status")
	}
	switch status.Status {
	case "IN_QUEUE":
		return "pending", nil, nil
	case "IN_PROGRESS":
		return "running", nil, nil
	case "COMPLETED":
		data, err := c.request(ctx, "GET", path, nil)
		if err != nil {
			var rejected *HTTPError
			if errors.As(err, &rejected) && rejected.Status == 422 {
				return "failed", nil, err
			}
			return "", nil, err
		}
		return "completed", data, nil
	default:
		return "", nil, fmt.Errorf("unknown fal queue status %q", status.Status)
	}
}
func (c *FalClient) CreateVideo(ctx context.Context, p map[string]any) (string, error) {
	if err := c.ValidateVideo(ctx, p); err != nil {
		return "", err
	}
	out := map[string]any{"prompt": p["prompt"]}
	for _, k := range []string{"aspect_ratio", "resolution", "generate_audio", "seed"} {
		if v, ok := p[k]; ok {
			out[k] = v
		}
	}
	if d, ok := p["duration"]; ok {
		switch n := d.(type) {
		case int:
			out["duration"] = strconv.Itoa(n)
		case float64:
			out["duration"] = strconv.Itoa(int(n))
		}
	}
	frames, _ := videoFrames(p)
	for _, f := range frames {
		if f.FrameType == "first_frame" {
			out["image_url"] = f.ImageURL.URL
		} else {
			out["end_image_url"] = f.ImageURL.URL
		}
	}
	return c.submit(ctx, stringValue(p["model"]), out)
}
func (c *FalClient) GetVideo(ctx context.Context, id string) (string, []string, error) {
	status, data, err := c.result(ctx, c.model, id)
	if err != nil || status != "completed" {
		return status, nil, err
	}
	var result struct {
		Video struct {
			URL string `json:"url"`
		} `json:"video"`
	}
	if json.Unmarshal(data, &result) != nil || result.Video.URL == "" {
		return "", nil, fmt.Errorf("fal completed task result could not be retrieved: missing or invalid video URL")
	}
	return status, []string{result.Video.URL}, nil
}
func (c *FalClient) VideoContent(ctx context.Context, id string, index int) ([]byte, error) {
	return videoBytes(ctx, c, id, index, c.base)
}
func (c *FalClient) GenerateImages(ctx context.Context, p map[string]any) ([]GeneratedImage, error) {
	if err := c.ValidateImage(ctx, p); err != nil {
		return nil, err
	}
	model := stringValue(p["model"])
	refs, _ := imageReferences(p)
	out := map[string]any{"prompt": p["prompt"], "output_format": "png", "num_images": 1}
	if a, ok := p["aspect_ratio"]; ok {
		out["aspect_ratio"] = a
	}
	if len(refs) > 0 {
		out["image_urls"] = refs
		if !strings.HasSuffix(model, "/edit") {
			model += "/edit"
		}
	}
	id, err := c.submit(ctx, model, out)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	for {
		status, data, err := c.result(ctx, model, id)
		if err != nil {
			return nil, err
		}
		if status == "completed" {
			var res struct {
				Images []struct {
					URL string `json:"url"`
				} `json:"images"`
			}
			if json.Unmarshal(data, &res) != nil || len(res.Images) == 0 {
				return nil, fmt.Errorf("fal result contains no images")
			}
			images := []GeneratedImage{}
			for _, image := range res.Images {
				data, err := publicMedia(ctx, image.URL, 32<<20, c.base)
				if err != nil {
					return nil, err
				}
				kind := http.DetectContentType(data)
				if !strings.HasPrefix(kind, "image/") {
					return nil, fmt.Errorf("fal returned non-image data")
				}
				images = append(images, GeneratedImage{Data: data, MediaType: kind})
			}
			return images, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.pollInterval):
		}
	}
}
