package providers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"videoflow-go/internal/costs"
)

// HostClient talks to the services that resell Seedance video. Each one has its own
// request shape, written here from that service's public API documentation.
type HostClient struct {
	mediaHTTP
	host *costs.Host
}

// VideoHost reports whether a provider name is one of the Seedance resellers handled here.
func VideoHost(name string) bool {
	switch name {
	case "argolink", "loopling", "replicate", "kie", "wavespeed", "runway":
		return true
	}
	return false
}

func NewHostClient(name, key, base string) *HostClient {
	host := costs.HostByName(name)
	c := &HostClient{newMediaHTTP(key, base, host.Label, "Bearer "), host}
	if name == "runway" {
		c.headers = map[string]string{"X-Runway-Version": "2024-11-06"}
	}
	return c
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
		if c.host.Frames {
			m.FrameImages = []string{"first_frame", "last_frame"}
			m.Architecture.Input = append(m.Architecture.Input, "image")
		}
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
	if len(frames) > 0 && !c.host.Frames {
		return fmt.Errorf("%s cannot start a clip from a picture yet; choose another video service for storyboard scenes", c.host.Label)
	}
	if err = m.validateVideo(p); err != nil {
		return err
	}
	for key := range p {
		switch key {
		case "model", "prompt", "duration", "aspect_ratio", "resolution", "generate_audio", "seed", "frame_images":
		default:
			return fmt.Errorf("%s does not support video field %s", c.host.Label, key)
		}
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

func hostSeconds(p map[string]any) int {
	switch n := p["duration"].(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 5
}

// runwayRatios are the exact frame sizes Runway accepts for Seedance.
var runwayRatios = map[string]map[string]string{
	"480p":  {"21:9": "992:432", "16:9": "864:496", "4:3": "752:560", "1:1": "640:640", "3:4": "560:752", "9:16": "496:864"},
	"720p":  {"21:9": "1470:630", "16:9": "1280:720", "4:3": "1112:834", "1:1": "960:960", "3:4": "834:1112", "9:16": "720:1280"},
	"1080p": {"21:9": "2206:946", "16:9": "1920:1080", "4:3": "1664:1248", "1:1": "1440:1440", "3:4": "1248:1664", "9:16": "1080:1920"},
}

// runwayRatioFor holds the sizes a model takes in place of the ones above.
var runwayRatioFor = map[string]map[string]string{"seedance2_5": {"864:496": "854:480", "496:864": "480:854"}}

func (c *HostClient) CreateVideo(ctx context.Context, p map[string]any) (string, error) {
	if err := c.ValidateVideo(ctx, p); err != nil {
		return "", err
	}
	frames, _ := videoFrames(p)
	first, last := "", ""
	for _, f := range frames {
		link, err := c.publicImage(ctx, f.ImageURL.URL, stringValue(p["model"]))
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
	resolution, aspect := stringValue(p["resolution"]), stringValue(p["aspect_ratio"])
	if resolution == "" {
		resolution = "720p"
	}
	set := func(m map[string]any, pairs ...any) map[string]any {
		for i := 0; i+1 < len(pairs); i += 2 {
			if s, isText := pairs[i+1].(string); pairs[i+1] != nil && !(isText && s == "") {
				m[pairs[i].(string)] = pairs[i+1]
			}
		}
		return m
	}
	common := func() map[string]any {
		return set(map[string]any{"prompt": prompt, "duration": hostSeconds(p), "resolution": resolution}, "aspect_ratio", aspect, "generate_audio", p["generate_audio"])
	}
	path, idField := "", "id"
	var body map[string]any
	switch c.host.Name {
	case "argolink":
		body, path, idField = common(), "/videos/generations", "request_id"
		body["model"] = model
		if first != "" {
			body["start_image"] = map[string]string{"url": first}
		}
		if last != "" {
			body["end_image"] = map[string]string{"url": last}
		}
	case "loopling":
		body, path = common(), "/video/generations"
		body["model"] = model
	case "replicate":
		body, path = map[string]any{"input": set(common(), "image", first, "last_frame_image", last, "seed", p["seed"])}, "/models/"+model+"/predictions"
	case "kie":
		body, path, idField = map[string]any{"model": model, "input": set(common(), "first_frame_url", first, "last_frame_url", last)}, "/v1/jobs/createTask", "data.taskId"
	case "wavespeed":
		mode := "/text-to-video"
		if first != "" {
			mode = "/image-to-video"
		}
		body, path, idField = set(common(), "image", first, "last_image", last), "/"+model+mode, "data.id"
	case "runway":
		if aspect == "" {
			aspect = "16:9"
		}
		ratio := runwayRatios[resolution][aspect]
		if own := runwayRatioFor[model][ratio]; own != "" {
			ratio = own
		}
		if ratio == "" {
			return "", fmt.Errorf("Runway does not offer %s at %s", aspect, resolution)
		}
		body = set(map[string]any{"model": model, "promptText": prompt, "duration": hostSeconds(p), "ratio": ratio}, "audio", p["generate_audio"], "seed", p["seed"])
		path = "/text_to_video"
		if first != "" {
			path = "/image_to_video"
			images := []map[string]string{{"uri": first, "position": "first"}}
			if last != "" {
				images = append(images, map[string]string{"uri": last, "position": "last"})
			}
			body["promptImage"] = images
		}
	default:
		return "", fmt.Errorf("unsupported video service %s", c.host.Name)
	}
	raw, err := c.request(ctx, "POST", path, body)
	if err != nil {
		return "", err
	}
	if err = c.rejected(raw); err != nil {
		return "", err
	}
	id := jsonText(raw, idField)
	if id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", fmt.Errorf("%s returned no job ID", c.host.Label)
	}
	return id, nil
}

// rejected reads the error some services put inside an HTTP 200 answer.
func (c *HostClient) rejected(raw []byte) error {
	if c.host.Name != "kie" {
		return nil
	}
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Code != 0 && envelope.Code != 200 {
		return &HTTPError{Provider: c.host.Label, Status: envelope.Code, Message: envelope.Msg}
	}
	return nil
}

// KeyState is what a video service last said about a saved key.
type KeyState string

const (
	KeyAccepted  KeyState = "accepted"
	KeyRejected  KeyState = "rejected"
	KeyUnchecked KeyState = "unchecked" // the service could not be asked
)

type KeyCheck struct {
	State     KeyState  `json:"state"`
	Message   string    `json:"message,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// keyCheckPaths are free, read-only requests that need a valid key. None of them makes
// or bills anything. Each is taken from the service's own API reference:
var keyCheckPaths = map[string]string{
	"argolink":  "/account/balance", // argolink.io/docs: GET /v1/account/balance; bad key is 401 INVALID_API_KEY
	"loopling":  "/me",              // loopling.ai/seedance/docs/get-account and /errors: bad key is 401 invalid_api_key
	"replicate": "/account",         // replicate.com/docs/reference/http#account.get
	"kie":       "/v1/chat/credit",  // docs.kie.ai/common-api/get-account-credits: bad key is HTTP 200 with code 401
	"wavespeed": "/balance",         // wavespeed.ai/docs/docs-common-api/balance; bad key is 401 (docs/api-authentication)
	"runway":    "/organization",    // Runway API reference, Organization: GET /v1/organization
}

// RejectedKey is the answer recorded when a service refuses its key.
func RejectedKey(label string) KeyCheck {
	return KeyCheck{KeyRejected, label + " did not accept this key. Copy the API key from your " + label + " dashboard and paste it again.", time.Now().UTC()}
}

// InvalidKey reports whether a service refused a request because of its key: HTTP 401,
// or 403 with a message about the key (a bare 403 can mean the account needs a top-up).
func InvalidKey(err error) bool {
	var e *HTTPError
	if !errors.As(err, &e) {
		return false
	}
	message := strings.ToLower(e.Message)
	return e.Status == 401 || e.Status == 403 && (strings.Contains(message, "api key") || strings.Contains(message, "api_key") || strings.Contains(message, "invalid key") || strings.Contains(message, "unauthori") || strings.Contains(message, "authenticat"))
}

// CheckKey asks the service whether it accepts the key, without making anything.
func (c *HostClient) CheckKey(ctx context.Context) KeyCheck {
	label := c.host.Label
	if strings.HasPrefix(c.key, "whsec_") {
		return KeyCheck{KeyRejected, "This is a webhook signing secret, not an API key. Copy the API key from your " + label + " dashboard and paste it again.", time.Now().UTC()}
	}
	raw, err := c.request(ctx, "GET", keyCheckPaths[c.host.Name], nil)
	if err == nil {
		err = c.rejected(raw)
	}
	switch {
	case err == nil:
		return KeyCheck{KeyAccepted, "", time.Now().UTC()}
	case InvalidKey(err):
		return RejectedKey(label)
	}
	return KeyCheck{KeyUnchecked, "This key could not be checked with " + label + " right now. It is saved, and the app will check it again later.", time.Now().UTC()}
}

func (c *HostClient) GetVideo(ctx context.Context, id string) (string, []string, error) {
	if id == "" || strings.ContainsAny(id, "/\\?#") {
		return "", nil, fmt.Errorf("invalid %s job ID", c.host.Label)
	}
	safe := url.PathEscape(id)
	path, statusField, urlField, errorField := "", "status", "", "error"
	switch c.host.Name {
	case "argolink":
		path, urlField = "/videos/"+safe, "video.url"
	case "loopling":
		path = "/video/generations/" + safe
	case "replicate":
		path, urlField = "/predictions/"+safe, "output"
	case "kie":
		path, statusField, errorField = "/v1/jobs/recordInfo?taskId="+url.QueryEscape(id), "data.state", "data.failMsg"
	case "wavespeed":
		path, statusField, urlField, errorField = "/predictions/"+safe+"/result", "data.status", "data.outputs", "data.error"
	case "runway":
		path, urlField, errorField = "/tasks/"+safe, "output", "failure"
	}
	raw, err := c.request(ctx, "GET", path, nil)
	if err != nil {
		return "", nil, err
	}
	if err = c.rejected(raw); err != nil {
		return "", nil, err
	}
	switch strings.ToLower(jsonText(raw, statusField)) {
	case "pending", "queued", "waiting", "queuing", "created", "starting", "throttled":
		return "pending", nil, nil
	case "running", "processing", "generating", "in_progress":
		return "running", nil, nil
	case "done", "succeeded", "success", "completed":
	case "failed", "fail", "expired", "canceled", "cancelled", "nsfw", "timeout", "deleted":
		message := jsonText(raw, errorField)
		if message == "" {
			message = jsonText(raw, errorField+".message")
		}
		if c.key != "" {
			message = strings.ReplaceAll(message, c.key, "[redacted]")
		}
		return "failed", nil, fmt.Errorf("%s could not make the video (%s): %s", c.host.Label, jsonText(raw, statusField), message)
	default:
		return "", nil, fmt.Errorf("unknown %s job status %q", c.host.Label, jsonText(raw, statusField))
	}
	link := ""
	switch c.host.Name {
	case "kie":
		var result struct {
			URLs []string `json:"resultUrls"`
		}
		if json.Unmarshal([]byte(jsonText(raw, "data.resultJson")), &result) == nil && len(result.URLs) > 0 {
			link = result.URLs[0]
		}
	case "loopling":
		// The finished file is behind a redirect that needs the key; resolve it to the file's own address.
		link, err = c.redirectTarget(ctx, "/video/generations/"+safe+"/content")
		if err != nil {
			return "", nil, err
		}
	default:
		link = jsonText(raw, urlField)
	}
	if link == "" {
		return "", nil, fmt.Errorf("%s finished the job but the result could not be retrieved: missing video URL", c.host.Label)
	}
	return "completed", []string{link}, nil
}

func (c *HostClient) VideoContent(ctx context.Context, id string, index int) ([]byte, error) {
	return videoBytes(ctx, c, id, index, c.base)
}

func (c *HostClient) redirectTarget(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", c.auth+c.key)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s request failed: %w", c.host.Label, err)
	}
	defer res.Body.Close()
	if target := res.Header.Get("Location"); res.StatusCode >= 300 && res.StatusCode < 400 && target != "" {
		return target, nil
	}
	return "", fmt.Errorf("%s did not return the finished video (HTTP %d)", c.host.Label, res.StatusCode)
}

// publicImage gives the service a picture it can read. Pictures made in the app are held
// as inline data; services that only accept web addresses get the picture uploaded to
// their own storage first.
func (c *HostClient) publicImage(ctx context.Context, image, model string) (string, error) {
	if !strings.HasPrefix(image, "data:") {
		return image, nil
	}
	if c.host.Name == "runway" || c.host.Name == "replicate" {
		return image, nil // both accept inline pictures
	}
	header, encoded, ok := strings.Cut(strings.TrimPrefix(image, "data:"), ",")
	kind := strings.TrimSuffix(header, ";base64")
	if !ok || kind == header || !strings.HasPrefix(kind, "image/") {
		return "", fmt.Errorf("picture is not in a format %s can read", c.host.Label)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("picture data is damaged")
	}
	name := "frame." + strings.TrimPrefix(kind, "image/")
	switch c.host.Name {
	case "kie":
		raw, err := c.request(ctx, "POST", "/file-base64-upload", map[string]any{"base64Data": image, "uploadPath": "videoflow", "fileName": strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + name})
		if err != nil {
			return "", err
		}
		return needLink(c, jsonText(raw, "data.downloadUrl"))
	case "wavespeed":
		var form bytes.Buffer
		writer := multipart.NewWriter(&form)
		part, _ := writer.CreateFormFile("file", name)
		_, _ = part.Write(data)
		_ = writer.Close()
		raw, err := c.requestBody(ctx, "POST", "/media/upload/binary", writer.FormDataContentType(), &form)
		if err != nil {
			return "", err
		}
		link := jsonText(raw, "data.download_url")
		if link == "" {
			link = jsonText(raw, "data.url")
		}
		return needLink(c, link)
	case "argolink":
		raw, err := c.request(ctx, "POST", "/media/uploads", map[string]any{"model": model, "type": "image", "content_type": kind, "size_bytes": len(data)})
		if err != nil {
			return "", err
		}
		if err = putBytes(ctx, jsonText(raw, "upload_url"), kind, data); err != nil {
			return "", fmt.Errorf("%s picture upload failed: %w", c.host.Label, err)
		}
		return needLink(c, jsonText(raw, "media_url"))
	}
	return "", fmt.Errorf("%s cannot start a clip from a picture yet", c.host.Label)
}

func needLink(c *HostClient, link string) (string, error) {
	if link == "" {
		return "", fmt.Errorf("%s accepted the picture but returned no address for it", c.host.Label)
	}
	return link, nil
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
// first text item, so "output" works whether a service returns one address or several.
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
