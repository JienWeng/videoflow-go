package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"strings"
	"time"
)

// MediaClient is the server's canonical media boundary; text stays on OpenRouter.
type MediaClient interface {
	Models(context.Context, string) ([]Model, error)
	Model(context.Context, string, string) (*Model, error)
	ValidateImage(context.Context, map[string]any) error
	GenerateImages(context.Context, map[string]any) ([]GeneratedImage, error)
	ValidateVideo(context.Context, map[string]any) error
	CreateVideo(context.Context, map[string]any) (string, error)
	GetVideo(context.Context, string) (string, []string, error)
	VideoContent(context.Context, string, int) ([]byte, error)
}

const FalURL = "https://queue.fal.run"
const BytePlusURL = "https://ark.ap-southeast.bytepluses.com/api/v3"
const FalImageModel = "fal-ai/nano-banana-2"
const FalVideoModel = "bytedance/seedance-2.0/fast/image-to-video"
const BytePlusVideoModel = "dreamina-seedance-2-0-260128"

type mediaHTTP struct {
	key, base, provider, auth string
	client                    *http.Client
}

func newMediaHTTP(key, base, provider, auth string) mediaHTTP {
	return mediaHTTP{key: key, base: strings.TrimRight(base, "/"), provider: provider, auth: auth, client: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return fmt.Errorf("provider API redirects are not followed")
	}}}
}
func (c *mediaHTTP) request(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.auth+c.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s request failed: %w", c.provider, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("%s response too large", c.provider)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := string(data)
		if c.key != "" {
			message = strings.ReplaceAll(message, c.key, "[redacted]")
		}
		if len(message) > 1000 {
			message = message[:1000]
		}
		return nil, &HTTPError{Provider: c.provider, Status: res.StatusCode, Message: message}
	}
	return data, nil
}
func mediaCatalog(provider, modality string) []Model {
	models := []Model{}
	add := func(id, kind string, frames bool) {
		m := Model{ID: id, Name: id}
		m.Architecture.Output = []string{kind}
		m.Architecture.Input = []string{"text"}
		m.AspectRatios = []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}
		if kind == "video" {
			for n := 4; n <= 15; n++ {
				m.Durations = append(m.Durations, n)
			}
			m.Resolutions = []string{"480p", "720p"}
			m.GenerateAudio = true
			if frames {
				m.FrameImages = []string{"first_frame", "last_frame"}
				m.Architecture.Input = append(m.Architecture.Input, "image")
			}
		}
		if kind == "image" {
			m.AspectRatios = []string{"auto", "21:9", "16:9", "3:2", "4:3", "5:4", "1:1", "4:5", "3:4", "2:3", "9:16"}
			m.Architecture.Input = append(m.Architecture.Input, "image")
			m.SupportedParameters, _ = json.Marshal(map[string]any{"aspect_ratio": map[string]any{"type": "enum", "values": m.AspectRatios}, "input_references": map[string]any{"type": "range", "min": 1, "max": 14}})
		}
		if modality == kind {
			models = append(models, m)
		}
	}
	switch provider {
	case "fal":
		add(FalImageModel, "image", false)
		add(FalImageModel+"/edit", "image", false)
		add(FalVideoModel, "video", true)
		add("bytedance/seedance-2.0/fast/text-to-video", "video", false)
	case "byteplus":
		add(BytePlusVideoModel, "video", true)
	}
	return models
}
func catalogModel(provider, id, kind string) (*Model, error) {
	for _, m := range mediaCatalog(provider, kind) {
		if m.ID == id {
			return &m, nil
		}
	}
	return nil, fmt.Errorf("%s model %q is not a supported %s endpoint", provider, id, kind)
}
func imageReferences(payload map[string]any) ([]string, error) {
	raw, ok := payload["input_references"]
	if !ok {
		return nil, nil
	}
	data, _ := json.Marshal(raw)
	var refs []struct {
		ImageURL struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if json.Unmarshal(data, &refs) != nil || len(refs) == 0 {
		return nil, fmt.Errorf("invalid image references")
	}
	urls := []string{}
	for _, r := range refs {
		if strings.TrimSpace(r.ImageURL.URL) == "" {
			return nil, fmt.Errorf("reference image URL is required")
		}
		urls = append(urls, r.ImageURL.URL)
	}
	return urls, nil
}

type canonicalFrame struct {
	FrameType string `json:"frame_type"`
	ImageURL  struct {
		URL string `json:"url"`
	} `json:"image_url"`
}

func videoFrames(payload map[string]any) ([]canonicalFrame, error) {
	var frames []canonicalFrame
	if raw, ok := payload["frame_images"]; ok {
		data, _ := json.Marshal(raw)
		if json.Unmarshal(data, &frames) != nil {
			return nil, fmt.Errorf("invalid frame images")
		}
	}
	for _, f := range frames {
		if strings.TrimSpace(f.ImageURL.URL) == "" {
			return nil, fmt.Errorf("frame URL is required")
		}
	}
	return frames, nil
}
func validateMediaVideo(provider string, ctx context.Context, p map[string]any) error {
	id, _ := p["model"].(string)
	m, err := catalogModel(provider, id, "video")
	if err != nil {
		return err
	}
	if err = m.validateVideo(p); err != nil {
		return err
	}
	for key := range p {
		switch key {
		case "model", "prompt", "duration", "aspect_ratio", "resolution", "generate_audio", "seed", "frame_images":
		default:
			return fmt.Errorf("%s does not support video field %s", provider, key)
		}
	}
	frames, err := videoFrames(p)
	if err != nil {
		return err
	}
	first := false
	for _, f := range frames {
		first = first || f.FrameType == "first_frame"
	}
	if provider == "fal" && strings.HasSuffix(id, "/image-to-video") && !first {
		return fmt.Errorf("selected fal model requires a first-frame image; generate a keyframe or select text-to-video")
	}
	if len(frames) > 0 && !first {
		return fmt.Errorf("a last frame requires a first frame")
	}
	return nil
}
func videoBytes(ctx context.Context, c MediaClient, id string, index int, base string) ([]byte, error) {
	_, urls, err := c.GetVideo(ctx, id)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(urls) {
		return nil, fmt.Errorf("provider returned no downloadable video")
	}
	return publicMedia(ctx, urls[index], 64<<20, base)
}
