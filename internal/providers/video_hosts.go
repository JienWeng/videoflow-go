package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"videoflow-go/internal/costs"
)

// HostClient talks to ArgoLink, the Seedance reseller, in the request shape of its
// public API documentation.
type HostClient struct {
	mediaHTTP
	host *costs.Host
}

// VideoHost reports whether a provider name is the Seedance reseller handled here.
func VideoHost(name string) bool { return name == "argolink" }

func NewHostClient(name, key, base string) *HostClient {
	host := costs.HostByName(name)
	return &HostClient{newMediaHTTP(key, base, host.Label, "Bearer "), host}
}

func (c *HostClient) Models(_ context.Context, kind string) ([]Model, error) {
	out := []Model{}
	if kind != "video" {
		return out, nil
	}
	ids := []string{}
	for family := range c.host.Models {
		ids = append(ids, family)
	}
	sort.Strings(ids)
	for _, family := range ids {
		m := Model{ID: c.host.Models[family], Name: c.host.Label + " · " + family, GenerateAudio: true}
		m.Architecture.Output, m.Architecture.Input = []string{"video"}, []string{"text"}
		m.AspectRatios = []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}
		for n := 4; n <= 15; n++ {
			m.Durations = append(m.Durations, n)
		}
		for _, resolution := range []string{"480p", "720p", "1080p"} {
			if _, ok := costs.HostPrice(c.host.Name, family, resolution, "16:9", nil); ok {
				m.Resolutions = append(m.Resolutions, resolution)
			}
		}
		if len(m.Resolutions) == 0 {
			m.Resolutions = []string{"480p", "720p"}
		}
		m.FrameImages = []string{"first_frame", "last_frame"}
		m.Architecture.Input = append(m.Architecture.Input, "image")
		m.ReferenceImages = c.host.References
		m.ReferenceAudios = c.host.Voices
		out = append(out, m)
	}
	return out, nil
}

func (c *HostClient) Model(ctx context.Context, id, kind string) (*Model, error) {
	models, _ := c.Models(ctx, kind)
	for i := range models {
		if models[i].ID == id {
			return &models[i], nil
		}
	}
	return nil, fmt.Errorf("%s does not offer %s model %q", c.host.Label, kind, id)
}

func (c *HostClient) ValidateImage(context.Context, map[string]any) error {
	return fmt.Errorf("%s is connected for video only", c.host.Label)
}
func (c *HostClient) GenerateImages(ctx context.Context, p map[string]any) ([]GeneratedImage, error) {
	return nil, c.ValidateImage(ctx, p)
}

func (c *HostClient) ValidateVideo(ctx context.Context, p map[string]any) error {
	id, _ := p["model"].(string)
	m, err := c.Model(ctx, id, "video")
	if err != nil {
		return err
	}
	frames, err := videoFrames(p)
	if err != nil {
		return err
	}
	if err = m.validateVideo(p); err != nil {
		return err
	}
	for key := range p {
		switch key {
		case "model", "prompt", "duration", "aspect_ratio", "resolution", "generate_audio", "seed", "frame_images", "image_urls", "audio_urls":
		default:
			return fmt.Errorf("%s does not support video field %s", c.host.Label, key)
		}
	}
	refs, err := referenceImageURLs(p)
	if err != nil {
		return err
	}
	if len(refs) > m.ReferenceImages {
		return fmt.Errorf("%s takes at most %d reference pictures per clip", c.host.Label, m.ReferenceImages)
	}
	if voices := textList(p["audio_urls"]); len(voices) > m.ReferenceAudios {
		return fmt.Errorf("%s takes at most %d voice recordings per clip", c.host.Label, m.ReferenceAudios)
	} else if len(voices) > 0 && len(refs) == 0 {
		return fmt.Errorf("voice recordings are sent only with reference pictures")
	}
	if len(refs) > 0 && len(frames) > 0 {
		return fmt.Errorf("a clip is made either from reference pictures or from a first frame, not both")
	}
	first := false
	for _, f := range frames {
		first = first || f.FrameType == "first_frame"
	}
	if len(frames) > 0 && !first {
		return fmt.Errorf("a last frame requires a first frame")
	}
	return nil
}

// referenceImageURLs reads image_urls: the reference pictures of a clip, in the order the
// prompt numbers them.
func referenceImageURLs(p map[string]any) ([]string, error) {
	raw, ok := p["image_urls"]
	if !ok || raw == nil {
		return nil, nil
	}
	list := []string{}
	switch v := raw.(type) {
	case []string:
		list = v
	case []any:
		for _, item := range v {
			text, isText := item.(string)
			if !isText {
				return nil, fmt.Errorf("reference pictures must be addresses")
			}
			list = append(list, text)
		}
	default:
		return nil, fmt.Errorf("reference pictures must be a list")
	}
	for _, link := range list {
		if strings.TrimSpace(link) == "" {
			return nil, fmt.Errorf("a reference picture has no address")
		}
	}
	return list, nil
}

func hostSeconds(p map[string]any) int {
	switch n := p["duration"].(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 5
}

func (c *HostClient) CreateVideo(ctx context.Context, p map[string]any) (string, error) {
	if err := c.ValidateVideo(ctx, p); err != nil {
		return "", err
	}
	frames, _ := videoFrames(p)
	first, last := "", ""
	for _, f := range frames {
		link, err := c.publicMedia(ctx, f.ImageURL.URL, stringValue(p["model"]), "image")
		if err != nil {
			return "", err
		}
		if f.FrameType == "first_frame" {
			first = link
		} else {
			last = link
		}
	}
	model, prompt := stringValue(p["model"]), stringValue(p["prompt"])
	refs, _ := referenceImageURLs(p)
	references := []map[string]string{}
	for _, ref := range refs {
		link, err := c.publicMedia(ctx, ref, model, "image")
		if err != nil {
			return "", err
		}
		references = append(references, map[string]string{"url": link})
	}
	voices := []map[string]string{}
	for _, voice := range textList(p["audio_urls"]) {
		link, err := c.publicMedia(ctx, voice, model, "audio")
		if err != nil {
			return "", err
		}
		voices = append(voices, map[string]string{"url": link})
	}
	resolution, aspect := stringValue(p["resolution"]), stringValue(p["aspect_ratio"])
	if resolution == "" {
		resolution = "720p"
	}
	body := map[string]any{"model": model, "prompt": prompt, "duration": hostSeconds(p), "resolution": resolution}
	if aspect != "" {
		body["aspect_ratio"] = aspect
	}
	if audio := p["generate_audio"]; audio != nil {
		body["generate_audio"] = audio
	}
	if first != "" {
		body["start_image"] = map[string]string{"url": first}
	}
	if last != "" {
		body["end_image"] = map[string]string{"url": last}
	}
	if len(references) > 0 {
		body["reference_images"] = references // named @Image 1, @Image 2, ... in the prompt
	}
	if len(voices) > 0 {
		body["reference_audios"] = voices // named @Audio 1, @Audio 2, ... in the prompt
	}
	raw, err := c.request(ctx, "POST", "/videos/generations", body)
	if err != nil {
		return "", err
	}
	id := jsonText(raw, "request_id")
	if id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", fmt.Errorf("%s returned no job ID", c.host.Label)
	}
	return id, nil
}

// KeyState is what a video service last said about a saved key.
type KeyState string

const (
	KeyAccepted  KeyState = "accepted"
	KeyRejected  KeyState = "rejected"
	KeyUnchecked KeyState = "unchecked" // the service could not be asked
	KeyNoCredit  KeyState = "no_credit" // the key works but the account has no balance left
)

type KeyCheck struct {
	State     KeyState  `json:"state"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// RejectedKey is the answer recorded when a service refuses its key.
func RejectedKey(label string) KeyCheck {
	return KeyCheck{KeyRejected, label + " did not accept this key. Copy the API key from your " + label + " dashboard and paste it again.", time.Now().UTC()}
}

// OutOfCredit is the answer recorded when a service says the account has no balance left.
func OutOfCredit(label string) KeyCheck {
	return KeyCheck{KeyNoCredit, label + " says the account has no balance left. Top it up at " + label + ", then press Check again.", time.Now().UTC()}
}

// NoCredit reports whether a service refused a request because the account has no
// balance: HTTP 402, or a message such as fal.ai's 403 "User is locked. Reason: Exhausted balance".
func NoCredit(err error) bool {
	var e *HTTPError
	if !errors.As(err, &e) {
		return false
	}
	if e.Status == 402 {
		return true
	}
	message := strings.ToLower(e.Message)
	for _, phrase := range []string{"exhausted balance", "insufficient balance", "insufficient credit", "insufficient funds", "not enough credit", "out of credit", "no credits", "balance is not enough", "user is locked"} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

// InvalidKey reports whether a service refused a request because of its key: HTTP 401,
// or 403 with a message about the key (a bare 403 can mean the account needs a top-up).
// NoSuchRequest reports that the service answered a status question with "not found":
// it holds no job under that id.
func NoSuchRequest(err error) bool {
	var e *HTTPError
	return errors.As(err, &e) && e.Status == 404
}

func InvalidKey(err error) bool {
	var e *HTTPError
	if !errors.As(err, &e) || NoCredit(err) {
		return false
	}
	message := strings.ToLower(e.Message)
	// Google answers a bad key with 400 "API key not valid" (reason API_KEY_INVALID).
	if e.Status == 400 && (strings.Contains(message, "api key not valid") || strings.Contains(message, "api_key_invalid")) {
		return true
	}
	return e.Status == 401 || e.Status == 403 && (strings.Contains(message, "api key") || strings.Contains(message, "api_key") || strings.Contains(message, "invalid key") || strings.Contains(message, "unauthori") || strings.Contains(message, "authenticat"))
}

// CheckKey asks the service whether it accepts the key, without making or billing
// anything: argolink.io/docs, GET /v1/account/balance; a bad key is 401 INVALID_API_KEY.
func (c *HostClient) CheckKey(ctx context.Context) KeyCheck {
	label := c.host.Label
	if strings.HasPrefix(c.key, "whsec_") {
		return KeyCheck{KeyRejected, "This is a webhook signing secret, not an API key. Copy the API key from your " + label + " dashboard and paste it again.", time.Now().UTC()}
	}
	_, err := c.request(ctx, "GET", "/account/balance", nil)
	switch {
	case err == nil:
		return KeyCheck{KeyAccepted, "", time.Now().UTC()}
	case NoCredit(err):
		return OutOfCredit(label)
	case InvalidKey(err):
		return RejectedKey(label)
	}
	return KeyCheck{KeyUnchecked, "This key could not be checked with " + label + " right now. It is saved, and the app will check it again later.", time.Now().UTC()}
}

func (c *HostClient) GetVideo(ctx context.Context, id string) (string, []string, error) {
	if id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", nil, fmt.Errorf("invalid %s job ID", c.host.Label)
	}
	raw, err := c.request(ctx, "GET", "/videos/"+url.PathEscape(id), nil)
	if err != nil {
		return "", nil, err
	}
	switch strings.ToLower(jsonText(raw, "status")) {
	case "pending", "queued", "waiting", "queuing", "created", "starting", "throttled":
		return "pending", nil, nil
	case "running", "processing", "generating", "in_progress":
		return "running", nil, nil
	case "done", "succeeded", "success", "completed":
	case "failed", "fail", "expired", "canceled", "cancelled", "nsfw", "timeout", "deleted":
		message := jsonText(raw, "error")
		if message == "" {
			message = jsonText(raw, "error.message")
		}
		if c.key != "" {
			message = strings.ReplaceAll(message, c.key, "[redacted]")
		}
		return "failed", nil, fmt.Errorf("%s could not make the video (%s): %s", c.host.Label, jsonText(raw, "status"), message)
	default:
		return "", nil, fmt.Errorf("unknown %s job status %q", c.host.Label, jsonText(raw, "status"))
	}
	link := jsonText(raw, "video.url")
	if link == "" {
		return "", nil, fmt.Errorf("%s finished the job but the result could not be retrieved: missing video URL", c.host.Label)
	}
	return "completed", []string{link}, nil
}

func (c *HostClient) VideoContent(ctx context.Context, id string, index int) ([]byte, error) {
	return videoBytes(ctx, c, id, index, c.base)
}

// publicMedia gives the service an address for a picture or recording held as the file
// itself: it is uploaded to the service first. An address is passed through unchanged.
func (c *HostClient) publicMedia(ctx context.Context, media, model, kind string) (string, error) {
	if !strings.HasPrefix(media, "data:") {
		return media, nil
	}
	what := map[string]string{"image": "picture", "audio": "voice recording"}[kind]
	header, encoded, ok := strings.Cut(strings.TrimPrefix(media, "data:"), ",")
	mime := strings.TrimSuffix(header, ";base64")
	if !ok || mime == header || !strings.HasPrefix(mime, kind+"/") {
		return "", fmt.Errorf("%s is not in a format %s can read", what, c.host.Label)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%s data is damaged", what)
	}
	raw, err := c.request(ctx, "POST", "/media/uploads", map[string]any{"model": model, "type": kind, "content_type": mime, "size_bytes": len(data)})
	if err != nil {
		return "", err
	}
	if err = putBytes(ctx, jsonText(raw, "upload_url"), mime, data); err != nil {
		return "", fmt.Errorf("%s %s upload failed: %w", c.host.Label, what, err)
	}
	link := jsonText(raw, "media_url")
	if link == "" {
		return "", fmt.Errorf("%s accepted the %s but returned no address for it", c.host.Label, what)
	}
	return link, nil
}

// textList reads a list of strings from a request field.
func textList(v any) []string {
	out := []string{}
	switch list := v.(type) {
	case []string:
		return list
	case []any:
		for _, item := range list {
			if text, ok := item.(string); ok && text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

// putBytes sends a file to a one-time upload address. The service key is never sent there.
func putBytes(ctx context.Context, target, kind string, data []byte) error {
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "127.0.0.1")) {
		return fmt.Errorf("invalid upload address")
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", target, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", kind)
	res, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return nil
}

// jsonText reads a dotted path such as "data.id" from a JSON answer. A list yields its
// first text item.
func jsonText(raw []byte, path string) string {
	var value any
	if path == "" || json.Unmarshal(raw, &value) != nil {
		return ""
	}
	for _, key := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value = object[key]
	}
	if list, ok := value.([]any); ok && len(list) > 0 {
		value = list[0]
	}
	text, _ := value.(string)
	return text
}
