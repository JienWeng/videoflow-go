package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

const OpenAIURL = "https://api.openai.com/v1"
const OpenAIImageModel = "gpt-image-2"

var openAIImageSizes = map[string]string{
	"": "auto", "auto": "auto", "1:1": "1024x1024",
	"16:9": "1536x864", "9:16": "864x1536",
	"4:3": "1280x960", "3:4": "960x1280", "21:9": "1792x768",
}

type OpenAIImages struct {
	http  mediaHTTP
	model string
}

func NewOpenAIImages(key, endpoint, model string) *OpenAIImages {
	return &OpenAIImages{http: newMediaHTTP(key, endpoint, "OpenAI", "Bearer "), model: model}
}

func (c *OpenAIImages) Model(_ context.Context, id, modality string) (*Model, error) {
	if id != c.model || modality != "image" {
		return nil, fmt.Errorf("OpenAI image fallback supports only the configured image model")
	}
	m := &Model{ID: id, Name: id, SupportedParameters: json.RawMessage(`{"input_references":{"type":"array","max":16}}`)}
	m.Architecture.Input = []string{"text", "image"}
	m.Architecture.Output = []string{"image"}
	for aspect := range openAIImageSizes {
		if aspect != "" {
			m.AspectRatios = append(m.AspectRatios, aspect)
		}
	}
	return m, nil
}

func (c *OpenAIImages) Models(ctx context.Context, modality string) ([]Model, error) {
	m, err := c.Model(ctx, c.model, modality)
	if err != nil {
		return nil, err
	}
	return []Model{*m}, nil
}

func (c *OpenAIImages) ValidateImage(_ context.Context, payload map[string]any) error {
	aspect, _ := payload["aspect_ratio"].(string)
	if _, ok := openAIImageSizes[aspect]; !ok {
		return fmt.Errorf("unsupported OpenAI image aspect ratio %q", aspect)
	}
	prompt, _ := payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("image prompt is required")
	}
	refs, _ := payload["input_references"].([]map[string]any)
	if len(refs) > 16 {
		return fmt.Errorf("OpenAI image edits accept at most 16 references")
	}
	return nil
}

func (c *OpenAIImages) GenerateImages(ctx context.Context, payload map[string]any) ([]GeneratedImage, error) {
	if err := c.ValidateImage(ctx, payload); err != nil {
		return nil, err
	}
	aspect, _ := payload["aspect_ratio"].(string)
	prompt := payload["prompt"].(string)
	fields := map[string]string{"model": c.model, "prompt": prompt, "size": openAIImageSizes[aspect], "output_format": "png"}
	refs, _ := payload["input_references"].([]map[string]any)
	var response []byte
	var err error
	if len(refs) == 0 {
		response, err = c.http.request(ctx, http.MethodPost, "/images/generations", fields)
	} else {
		response, err = c.edit(ctx, fields, refs)
	}
	if err != nil {
		return nil, err
	}
	var result struct {
		Data []struct {
			Image string `json:"b64_json"`
		} `json:"data"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(response, &result) != nil || len(result.Data) != 1 {
		return nil, fmt.Errorf("OpenAI did not return one image")
	}
	image, err := base64.StdEncoding.DecodeString(result.Data[0].Image)
	if err != nil || http.DetectContentType(image) != "image/png" {
		return nil, fmt.Errorf("OpenAI returned invalid PNG data")
	}
	return []GeneratedImage{{Data: image, MediaType: "image/png", Usage: result.Usage}}, nil
}

func (c *OpenAIImages) edit(ctx context.Context, fields map[string]string, refs []map[string]any) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, err
		}
	}
	for i, ref := range refs {
		url, _ := ref["image_url"].(map[string]string)
		header, encoded, ok := strings.Cut(url["url"], ",")
		if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") || len(encoded) > base64.StdEncoding.EncodedLen(50<<20) {
			return nil, fmt.Errorf("OpenAI image edits require local raster references under 50 MiB")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("invalid image reference")
		}
		var ext string
		mediaType := http.DetectContentType(data)
		switch mediaType {
		case "image/png":
			ext = "png"
		case "image/jpeg":
			ext = "jpg"
		case "image/webp":
			ext = "webp"
		default:
			return nil, fmt.Errorf("OpenAI image edits require PNG, JPEG or WebP references")
		}
		if body.Len()+len(data) > 64<<20 {
			return nil, fmt.Errorf("OpenAI image references exceed 64 MiB combined")
		}
		// CreateFormFile always labels the part application/octet-stream, which the
		// images API rejects; send the real image type.
		partHeader := textproto.MIMEHeader{}
		partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="reference-%d.%s"`, i+1, ext))
		partHeader.Set("Content-Type", mediaType)
		part, err := writer.CreatePart(partHeader)
		if err != nil {
			return nil, err
		}
		if _, err = part.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return c.http.requestBody(ctx, http.MethodPost, "/images/edits", writer.FormDataContentType(), &body)
}

func (c *OpenAIImages) ValidateVideo(context.Context, map[string]any) error {
	return fmt.Errorf("OpenAI fallback is configured for images only")
}
func (c *OpenAIImages) CreateVideo(ctx context.Context, payload map[string]any) (string, error) {
	return "", c.ValidateVideo(ctx, payload)
}
func (c *OpenAIImages) GetVideo(ctx context.Context, _ string) (string, []string, error) {
	return "", nil, c.ValidateVideo(ctx, nil)
}
func (c *OpenAIImages) VideoContent(ctx context.Context, _ string, _ int) ([]byte, error) {
	return nil, c.ValidateVideo(ctx, nil)
}
