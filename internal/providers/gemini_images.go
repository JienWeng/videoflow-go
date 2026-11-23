package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Google Gemini image generation (Nano Banana) through the Gemini API's generateContent.
// Model ids, aspect ratios and reference limits were checked against
// https://aistudio.google.com/docs/image-generation, /docs/models and /docs/deprecations
// (formerly ai.google.dev) on 23 Nov 2026.
const GeminiURL = "https://generativelanguage.googleapis.com/v1beta"
const GeminiImageModel = "gemini-3-pro-image"

// GeminiImageModelInfo is one picture model the app offers from Google.
type GeminiImageModelInfo struct {
	ID    string
	Label string
	// AspectRatios the model accepts as imageConfig.aspectRatio.
	AspectRatios []string
	// References is how many reference pictures one request may carry.
	References int
}

var geminiTenRatios = []string{"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"}

// GeminiImageModels lists the offered models; the first is the default.
var GeminiImageModels = []GeminiImageModelInfo{
	// Up to 6 object, 5 character and 3 style pictures: 14 in all.
	{ID: GeminiImageModel, Label: "Nano Banana Pro", AspectRatios: geminiTenRatios, References: 14},
	// Up to 10 object and 4 character pictures: 14 in all.
	{ID: "gemini-3.1-flash-image", Label: "Nano Banana 2 (Flash)", AspectRatios: append(append([]string{}, geminiTenRatios...), "1:4", "4:1", "1:8", "8:1"), References: 14},
	// Google: "works best with up to 3 images as input". Shuts down no earlier than 15 Mar 2027.
	{ID: "gemini-2.5-flash-image", Label: "Nano Banana (Flash)", AspectRatios: geminiTenRatios, References: 3},
}

func geminiModelInfo(id string) *GeminiImageModelInfo {
	for i := range GeminiImageModels {
		if GeminiImageModels[i].ID == id {
			return &GeminiImageModels[i]
		}
	}
	return nil
}

type GeminiImages struct {
	http  mediaHTTP
	model string
}

func NewGeminiImages(key, endpoint, model string) *GeminiImages {
	h := newMediaHTTP(key, endpoint, "Google Gemini", "")
	h.header = "x-goog-api-key"
	return &GeminiImages{http: h, model: model}
}

func (c *GeminiImages) Model(_ context.Context, id, modality string) (*Model, error) {
	info := geminiModelInfo(id)
	if info == nil || modality != "image" {
		return nil, fmt.Errorf("Google Gemini model %q is not a supported image model", id)
	}
	m := &Model{ID: info.ID, Name: info.Label, AspectRatios: append([]string{}, info.AspectRatios...)}
	m.Architecture.Input = []string{"text", "image"}
	m.Architecture.Output = []string{"image"}
	m.SupportedParameters, _ = json.Marshal(map[string]any{
		"aspect_ratio":     map[string]any{"type": "enum", "values": m.AspectRatios},
		"input_references": map[string]any{"type": "range", "min": 1, "max": info.References},
	})
	return m, nil
}

func (c *GeminiImages) Models(ctx context.Context, modality string) ([]Model, error) {
	if modality != "image" && modality != "all" {
		return []Model{}, nil
	}
	models := []Model{}
	for _, info := range GeminiImageModels {
		m, _ := c.Model(ctx, info.ID, "image")
		models = append(models, *m)
	}
	return models, nil
}

func (c *GeminiImages) modelID(payload map[string]any) string {
	if id, _ := payload["model"].(string); id != "" {
		return id
	}
	return c.model
}

func (c *GeminiImages) ValidateImage(_ context.Context, payload map[string]any) error {
	info := geminiModelInfo(c.modelID(payload))
	if info == nil {
		return fmt.Errorf("Google Gemini model %q is not a supported image model", c.modelID(payload))
	}
	for key := range payload {
		switch key {
		case "model", "prompt", "provider", "aspect_ratio", "input_references":
		default:
			return fmt.Errorf("Google Gemini image models do not support %s", key)
		}
	}
	if aspect, _ := payload["aspect_ratio"].(string); aspect != "" && aspect != "auto" && !contains(info.AspectRatios, aspect) {
		return fmt.Errorf("unsupported aspect ratio %q for %s", aspect, info.ID)
	}
	if prompt, _ := payload["prompt"].(string); strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("image prompt is required")
	}
	refs, err := imageReferences(payload)
	if err != nil {
		return err
	}
	if len(refs) > info.References {
		return fmt.Errorf("%s accepts at most %d reference pictures", info.Label, info.References)
	}
	return nil
}

type geminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *geminiBlob `json:"inlineData,omitempty"`
	Thought    bool        `json:"thought,omitempty"`
}

// geminiReference turns a data: URL into an inline picture part.
func geminiReference(raw string) (geminiPart, error) {
	header, encoded, ok := strings.Cut(raw, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
		return geminiPart{}, fmt.Errorf("Google Gemini needs reference pictures stored in VideoFlow")
	}
	// Inline request data is capped well below 20 MB in total; keep each picture modest.
	if len(encoded) > base64.StdEncoding.EncodedLen(7<<20) {
		return geminiPart{}, fmt.Errorf("Google Gemini reference pictures must be under 7 MiB each")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return geminiPart{}, fmt.Errorf("invalid image reference")
	}
	mediaType := http.DetectContentType(data)
	switch mediaType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return geminiPart{}, fmt.Errorf("Google Gemini reference pictures must be PNG, JPEG or WebP")
	}
	return geminiPart{InlineData: &geminiBlob{MimeType: mediaType, Data: encoded}}, nil
}

// Readable reasons for Gemini's image finish and block codes.
var geminiReasons = map[string]string{
	"SAFETY":                   "it was blocked by Google's safety filters",
	"IMAGE_SAFETY":             "the picture was blocked by Google's safety filters",
	"PROHIBITED_CONTENT":       "Google flagged the request as prohibited content",
	"IMAGE_PROHIBITED_CONTENT": "Google flagged the picture as prohibited content",
	"BLOCKLIST":                "the prompt contains blocked terms",
	"IMAGE_RECITATION":         "the picture was too close to existing material",
	"RECITATION":               "the result was too close to existing material",
	"NO_IMAGE":                 "the model answered without a picture",
	"IMAGE_OTHER":              "the picture could not be made",
	"OTHER":                    "the request could not be completed",
}

func geminiReason(code string) string {
	if reason := geminiReasons[code]; reason != "" {
		return reason + " (" + code + ")"
	}
	return code
}

func (c *GeminiImages) GenerateImages(ctx context.Context, payload map[string]any) ([]GeneratedImage, error) {
	if err := c.ValidateImage(ctx, payload); err != nil {
		return nil, err
	}
	model := c.modelID(payload)
	prompt := payload["prompt"].(string)
	parts := []geminiPart{{Text: prompt}}
	refs, _ := imageReferences(payload)
	for _, ref := range refs {
		part, err := geminiReference(ref)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	config := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
	if aspect, _ := payload["aspect_ratio"].(string); aspect != "" && aspect != "auto" {
		config["imageConfig"] = map[string]any{"aspectRatio": aspect}
	}
	body := map[string]any{
		"contents":         []map[string]any{{"role": "user", "parts": parts}},
		"generationConfig": config,
	}
	response, err := c.http.request(ctx, http.MethodPost, "/models/"+model+":generateContent", body)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) {
			var detail struct {
				Error struct {
					Message string `json:"message"`
					Status  string `json:"status"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(httpErr.Message), &detail) == nil && detail.Error.Message != "" {
				httpErr.Message = strings.TrimSpace(detail.Error.Status + " " + detail.Error.Message)
			}
		}
		return nil, err
	}
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []geminiPart `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		UsageMetadata json.RawMessage `json:"usageMetadata"`
	}
	if err = json.Unmarshal(response, &result); err != nil {
		return nil, fmt.Errorf("Google Gemini returned an unreadable answer")
	}
	if code := result.PromptFeedback.BlockReason; code != "" {
		return nil, fmt.Errorf("Google Gemini refused the prompt: %s", geminiReason(code))
	}
	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("Google Gemini returned no picture")
	}
	candidate := result.Candidates[0]
	// Thinking may show draft pictures first; the final picture is the last non-thought one.
	var image *geminiBlob
	text := ""
	for _, part := range candidate.Content.Parts {
		if part.Thought {
			continue
		}
		if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "image/") {
			image = part.InlineData
		} else if part.Text != "" {
			text += part.Text
		}
	}
	if image == nil {
		reason := "the model answered without a picture"
		if code := candidate.FinishReason; code != "" && code != "STOP" {
			reason = geminiReason(code)
		}
		if text = strings.TrimSpace(text); text != "" {
			if len(text) > 300 {
				text = text[:300] + "…"
			}
			reason += ": " + text
		}
		return nil, fmt.Errorf("Google Gemini returned no picture because %s", reason)
	}
	data, err := base64.StdEncoding.DecodeString(image.Data)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("Google Gemini returned invalid picture data")
	}
	mediaType := http.DetectContentType(data)
	switch mediaType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return nil, fmt.Errorf("Google Gemini returned an unsupported picture type %q", mediaType)
	}
	return []GeneratedImage{{Data: data, MediaType: mediaType, Usage: result.UsageMetadata}}, nil
}

func (c *GeminiImages) ValidateVideo(context.Context, map[string]any) error {
	return fmt.Errorf("Google Gemini is configured for pictures only")
}
func (c *GeminiImages) CreateVideo(ctx context.Context, payload map[string]any) (string, error) {
	return "", c.ValidateVideo(ctx, payload)
}
func (c *GeminiImages) GetVideo(ctx context.Context, _ string) (string, []string, error) {
	return "", nil, c.ValidateVideo(ctx, nil)
}
func (c *GeminiImages) VideoContent(ctx context.Context, _ string, _ int) ([]byte, error) {
	return nil, c.ValidateVideo(ctx, nil)
}
