package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google Gemini video through the Interactions API (Gemini Omni Flash).
//
// Checked against https://aistudio.google.com/docs/omni, /docs/background-execution,
// /docs/deprecations and /api-reference/interactions-api/create-interaction on 23 Nov 2026.
// Veo is no longer offered: "The Veo 3.1 Preview in the Gemini API has been shut down"
// (22 Oct 2026); Google names gemini-omni-1.2-flash as its replacement.
//
// The request: POST /v1beta/interactions with
//
//	{"model", "input": "<prompt>" | [image parts..., {"type":"text"}],
//	 "generation_config": {"video_config": {"task": "image_to_video" | "reference_to_video"}},
//	 "response_format": {"type":"video", "duration":"8s", "aspect_ratio", "resolution"},
//	 "background": true, "store": true}
//
// then GET /v1beta/interactions/{id} until status leaves "in_progress". The finished
// clip is in steps[type=model_output].content[type=video]: inline base64 "data" (what a
// GET returns, per the Omni page) or a Files API "uri".
//
// Not stated by Google for Omni and therefore assumed: that "background": true is
// accepted (the background page lists "standard Gemini models (such as ...)"), and that
// a safety block ends as status "failed" or as a finished interaction with no video. A
// synchronous answer is handled too: create waits up to 10 minutes and keeps any clip
// it already carries.
const GeminiVideoModel = "gemini-omni-1.2-flash"

// geminiVideoMaxBytes caps one clip. A 15 s 4K clip is the largest Omni makes.
const geminiVideoMaxBytes = 200 << 20

// geminiInlineLimit keeps the request under Google's 20 MB inline request size.
const geminiInlineLimit = 15 << 20

func geminiVideoModel(id string) (*Model, error) {
	if id != GeminiVideoModel {
		return nil, fmt.Errorf("Google Gemini model %q is not a supported video model; use %s", id, GeminiVideoModel)
	}
	m := &Model{ID: id, Name: "Gemini Omni Flash", GenerateAudio: true}
	m.Architecture.Input = []string{"text", "image"}
	m.Architecture.Output = []string{"video"}
	// "any whole second from 3 to 15 seconds"; first and last frame: 3 to 10 seconds.
	for n := 3; n <= 15; n++ {
		m.Durations = append(m.Durations, n)
	}
	m.AspectRatios = []string{"16:9", "9:16"}
	// 1080p and 4k are upscaled by Google. All four are priced in costs.Hosts.
	m.Resolutions = []string{"360p", "720p", "1080p", "4k"}
	m.FrameImages = []string{"first_frame", "last_frame"}
	// "up to 6" reference images; audio references are not supported.
	m.ReferenceImages = 6
	return m, nil
}

func (c *GeminiImages) ValidateVideo(ctx context.Context, p map[string]any) error {
	_, _, err := c.videoInput(ctx, p, false)
	return err
}

// videoInput checks a request and, when fetch is set, builds the Interactions "input"
// and task. Nothing is sent to Google while checking.
func (c *GeminiImages) videoInput(ctx context.Context, p map[string]any, fetch bool) (input any, task string, err error) {
	m, err := geminiVideoModel(stringValue(p["model"]))
	if err != nil {
		return nil, "", err
	}
	for key := range p {
		switch key {
		case "model", "prompt", "duration", "aspect_ratio", "resolution", "generate_audio", "frame_images", "image_urls":
		case "seed":
			return nil, "", fmt.Errorf("Google Gemini video does not take a seed")
		case "audio_urls":
			return nil, "", fmt.Errorf("Google Gemini video does not take voice recordings")
		default:
			return nil, "", fmt.Errorf("Google Gemini video does not support video field %s", key)
		}
	}
	if err = m.validateVideo(p); err != nil {
		return nil, "", err
	}
	frames, err := videoFrames(p)
	if err != nil {
		return nil, "", err
	}
	refs, err := referenceImageURLs(p)
	if err != nil {
		return nil, "", err
	}
	if len(refs) > m.ReferenceImages {
		return nil, "", fmt.Errorf("Google Gemini takes at most %d reference pictures per clip", m.ReferenceImages)
	}
	if len(refs) > 0 && len(frames) > 0 {
		return nil, "", fmt.Errorf("a clip is made either from reference pictures or from a first frame, not both")
	}
	var first, last string
	for _, f := range frames {
		if f.FrameType == "first_frame" {
			first = f.ImageURL.URL
		} else {
			last = f.ImageURL.URL
		}
	}
	if last != "" && first == "" {
		return nil, "", fmt.Errorf("a last frame requires a first frame")
	}
	if last != "" && hostSeconds(p) > 10 {
		return nil, "", fmt.Errorf("Google Gemini makes clips from a first and last frame of at most 10 seconds")
	}
	pictures := refs
	if first != "" {
		pictures, task = []string{first}, "image_to_video"
		if last != "" {
			pictures = append(pictures, last) // "the optional second image defines the ending frame"
		}
	} else if len(refs) > 0 {
		task = "reference_to_video"
	}
	prompt := strings.TrimSpace(stringValue(p["prompt"]))
	if len(pictures) == 0 {
		return prompt, "", nil
	}
	parts, total := []map[string]string{}, 0
	for _, picture := range pictures {
		part, err := c.videoPicture(ctx, picture, fetch)
		if err != nil {
			return nil, "", err
		}
		total += len(part["data"])
		if total > geminiInlineLimit {
			return nil, "", fmt.Errorf("the pictures for this clip are too large to send to Google Gemini together (15 MB at most)")
		}
		if len(part) > 0 {
			parts = append(parts, part)
		}
	}
	return append(parts, map[string]string{"type": "text", "text": prompt}), task, nil
}

// videoPicture turns a stored picture (data: URL) into an inline image part. A web
// address is fetched without the key only when fetch is set.
func (c *GeminiImages) videoPicture(ctx context.Context, picture string, fetch bool) (map[string]string, error) {
	encoded := ""
	if strings.HasPrefix(picture, "data:") {
		header, data, ok := strings.Cut(picture, ",")
		if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
			return nil, fmt.Errorf("Google Gemini needs pictures stored in VideoFlow or at a web address")
		}
		encoded = data
	} else if !strings.HasPrefix(picture, "https://") {
		return nil, fmt.Errorf("Google Gemini needs pictures stored in VideoFlow or at a web address")
	} else if !fetch {
		return map[string]string{}, nil // checked when it is fetched
	} else {
		data, err := publicMedia(ctx, picture, 10<<20, c.http.base)
		if err != nil {
			return nil, err
		}
		encoded = base64.StdEncoding.EncodeToString(data)
	}
	if len(encoded) > geminiInlineLimit {
		return nil, fmt.Errorf("a picture for this clip is too large to send to Google Gemini (15 MB at most)")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("a picture for this clip is damaged")
	}
	kind := http.DetectContentType(data)
	switch kind {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return nil, fmt.Errorf("Google Gemini video pictures must be PNG, JPEG or WebP")
	}
	return map[string]string{"type": "image", "data": encoded, "mime_type": kind}, nil
}

func (c *GeminiImages) CreateVideo(ctx context.Context, p map[string]any) (string, error) {
	if err := c.ValidateVideo(ctx, p); err != nil {
		return "", err
	}
	input, task, err := c.videoInput(ctx, p, true)
	if err != nil {
		return "", err
	}
	format := map[string]any{"type": "video"}
	if _, ok := p["duration"]; ok {
		format["duration"] = fmt.Sprintf("%ds", hostSeconds(p))
	}
	for _, key := range []string{"aspect_ratio", "resolution"} {
		if value := stringValue(p[key]); value != "" {
			format[key] = value
		}
	}
	body := map[string]any{"model": GeminiVideoModel, "input": input, "response_format": format, "background": true, "store": true}
	if task != "" {
		body["generation_config"] = map[string]any{"video_config": map[string]any{"task": task}}
	}
	// Should Google answer synchronously after all, the whole clip arrives in this answer.
	h := c.http
	h.limit = geminiVideoMaxBytes * 4 / 3
	h.client = &http.Client{Timeout: 10 * time.Minute, Transport: c.http.client.Transport, CheckRedirect: c.http.client.CheckRedirect}
	raw, err := h.request(ctx, http.MethodPost, "/interactions", body)
	if err != nil {
		return "", geminiHTTPError(err)
	}
	var answer geminiInteraction
	if json.Unmarshal(raw, &answer) != nil || !geminiSafeID(answer.ID) {
		return "", fmt.Errorf("Google Gemini returned no interaction ID")
	}
	if answer.Status == "completed" || answer.Status == "incomplete" {
		c.keepVideo(answer.ID, answer.video())
	}
	// A refused request still has an ID: polling reports why it failed.
	return answer.ID, nil
}

func geminiSafeID(id string) bool {
	return id != "" && len(id) <= 256 && !strings.ContainsAny(id, "/\\?#%")
}

type geminiContent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Data     string `json:"data"`
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
}

type geminiInteraction struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Steps  []struct {
		Type    string          `json:"type"`
		Content []geminiContent `json:"content"`
	} `json:"steps"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// geminiFinishedVideo is a finished clip: its bytes, or where Google keeps them.
type geminiFinishedVideo struct {
	id, data, uri string
}

func (i geminiInteraction) video() geminiFinishedVideo {
	out := geminiFinishedVideo{id: i.ID}
	for _, step := range i.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type == "video" && (content.Data != "" || content.URI != "") {
				out.data, out.uri = content.Data, content.URI
			}
		}
	}
	return out
}

// text is what the model said instead of a clip, shortened.
func (i geminiInteraction) text() string {
	text := ""
	for _, step := range i.Steps {
		for _, content := range step.Content {
			if content.Type == "text" {
				text += content.Text
			}
		}
	}
	text = strings.TrimSpace(text)
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	return text
}

func (c *GeminiImages) keepVideo(id string, video geminiFinishedVideo) {
	if video.data == "" && video.uri == "" {
		return
	}
	c.videoMu.Lock()
	defer c.videoMu.Unlock()
	video.id = id
	c.video = video
}

func (c *GeminiImages) keptVideo(id string) (geminiFinishedVideo, bool) {
	c.videoMu.Lock()
	defer c.videoMu.Unlock()
	return c.video, c.video.id == id
}

func (c *GeminiImages) GetVideo(ctx context.Context, id string) (string, []string, error) {
	if !geminiSafeID(id) {
		return "", nil, fmt.Errorf("invalid Google Gemini interaction ID")
	}
	h := c.http
	h.limit = geminiVideoMaxBytes * 4 / 3 // a finished interaction carries the clip as base64
	raw, err := h.request(ctx, http.MethodGet, "/interactions/"+url.PathEscape(id), nil)
	if err != nil {
		return "", nil, geminiHTTPError(err)
	}
	var answer geminiInteraction
	if json.Unmarshal(raw, &answer) != nil {
		return "", nil, fmt.Errorf("Google Gemini returned an unreadable interaction")
	}
	// The address is the interaction itself; it holds no key.
	link := []string{c.http.base + "/interactions/" + url.PathEscape(id)}
	switch answer.Status {
	case "in_progress":
		return "running", nil, nil
	case "completed", "incomplete":
		video := answer.video()
		if video.data == "" && video.uri == "" {
			reason := "Google Gemini finished without a video"
			if text := answer.text(); text != "" {
				reason += ": " + c.redact(text)
			} else {
				reason += "; it may have been blocked by Google's safety filters"
			}
			return "failed", nil, fmt.Errorf("%s", reason)
		}
		c.keepVideo(id, video)
		return "completed", link, nil
	case "failed", "cancelled":
		messages := []string{}
		for _, e := range answer.Errors {
			if message := strings.TrimSpace(e.Message); message != "" {
				messages = append(messages, c.redact(message))
			}
		}
		if text := answer.text(); text != "" {
			messages = append(messages, c.redact(text))
		}
		message := strings.Join(messages, "; ")
		if message == "" {
			message = "no reason given"
		}
		return "failed", nil, fmt.Errorf("Google Gemini could not make the video (%s): %s", answer.Status, message)
	case "requires_action", "requires_continuation":
		return "failed", nil, fmt.Errorf("Google Gemini stopped before finishing the video (%s)", answer.Status)
	}
	return "", nil, fmt.Errorf("unknown Google Gemini interaction status %q", answer.Status)
}

func (c *GeminiImages) redact(text string) string {
	if c.http.key != "" {
		return strings.ReplaceAll(text, c.http.key, "[redacted]")
	}
	return text
}

func (c *GeminiImages) VideoContent(ctx context.Context, id string, index int) ([]byte, error) {
	if index != 0 {
		return nil, fmt.Errorf("provider returned no downloadable video")
	}
	video, ok := c.keptVideo(id)
	if !ok {
		status, _, err := c.GetVideo(ctx, id)
		if err != nil {
			return nil, err
		}
		if status != "completed" {
			return nil, fmt.Errorf("Google Gemini has not finished this video")
		}
		video, _ = c.keptVideo(id)
	}
	var data []byte
	if video.data != "" {
		decoded, err := base64.StdEncoding.DecodeString(video.data)
		if err != nil {
			return nil, fmt.Errorf("Google Gemini returned damaged video data")
		}
		data = decoded
	} else {
		downloaded, err := c.downloadVideo(ctx, video.uri)
		if err != nil {
			return nil, err
		}
		data = downloaded
	}
	if len(data) == 0 || !strings.HasPrefix(http.DetectContentType(data), "video/") {
		return nil, fmt.Errorf("Google Gemini returned something other than a video")
	}
	c.videoMu.Lock()
	if c.video.id == id {
		c.video = geminiFinishedVideo{}
	}
	c.videoMu.Unlock()
	return data, nil
}

// downloadVideo fetches a Files API download address. The key goes in the header and only
// to Google's own API host; a redirect elsewhere is followed without it.
func (c *GeminiImages) downloadVideo(ctx context.Context, link string) ([]byte, error) {
	target, err := url.Parse(link)
	base, e := url.Parse(c.http.base)
	if err != nil || e != nil || target.Scheme != base.Scheme || target.Host != base.Host || target.User != nil || target.Fragment != "" || target.Query().Has("key") {
		return nil, fmt.Errorf("Google Gemini returned an unexpected video address")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", c.http.key)
	client := &http.Client{Timeout: 5 * time.Minute, Transport: c.http.client.Transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Gemini video download failed")
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		next, err := res.Location()
		if err != nil {
			return nil, fmt.Errorf("Google Gemini video download failed")
		}
		return publicMedia(ctx, next.String(), geminiVideoMaxBytes, c.http.base)
	}
	if res.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 1000))
		return nil, geminiHTTPError(&HTTPError{Provider: c.http.provider, Status: res.StatusCode, Message: c.redact(string(message))})
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, geminiVideoMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > geminiVideoMaxBytes {
		return nil, fmt.Errorf("Google Gemini video is too large")
	}
	return data, nil
}
