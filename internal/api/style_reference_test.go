package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestReferencePreviewAndPrivateLinks(t *testing.T) {
	got, e := referencePreview([]byte(`<meta content='/look.png?a=1&amp;b=2' property="og:image">`), "https://example.com/story")
	if e != nil || got != "https://example.com/look.png?a=1&b=2" {
		t.Fatal(got, e)
	}
	if _, e = referencePreview([]byte(`<script>fake image</script>`), "https://example.com"); e == nil {
		t.Fatal("page without preview accepted")
	}
	s := routerServer(t, nil)
	for _, link := range []string{"https://127.0.0.1/private", "https://169.254.169.254/latest", "https://100.76.131.81/private", "file:///etc/passwd", "https://name:secret@example.com/look.png"} {
		response := call(t, s, "POST", "/style/reference", string(voiceJSON(map[string]any{"url": link})))
		if response.Code == 200 {
			t.Fatal("unsafe reference accepted", link)
		}
	}
}

func TestReferenceAnalysisReturnsDraftWithoutSavingStyle(t *testing.T) {
	calls := 0
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]}}]}`))
			return
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected provider call %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		calls++
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "data:image/png;base64,") {
			t.Error("reference missing from vision input")
		}
		content := `{"name":"Watercolor","style_prompt":"Soft watercolor washes and simple shapes","palette":"Warm ochre","lighting":"Diffuse","tone":"Gentle","audience":"Families"}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": content}}}})
	})
	project, _ := s.database.GetActiveProjectID()
	before := call(t, s, "GET", "/style", "")
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "look.png"), data, 0600)
	asset := &models.Asset{ProjectID: &project, Name: "Look", Type: "other", FilePath: "storage/look.png"}
	if e := s.database.CreateAsset(asset); e != nil {
		t.Fatal(e)
	}
	response := call(t, s, "POST", "/style/reference", string(voiceJSON(map[string]any{"asset_id": asset.ID})))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"saved":false`) || calls != 1 {
		t.Fatal(response.Code, response.Body.String(), calls)
	}
	after := call(t, s, "GET", "/style", "")
	if before.Body.String() != after.Body.String() {
		t.Fatal("analysis overwrote project style")
	}
	other, e := s.database.CreateProject("Other", "")
	if e != nil {
		t.Fatal(e)
	}
	asset.ProjectID = &other.ID
	asset.ID = ""
	if e = s.database.CreateAsset(asset); e != nil {
		t.Fatal(e)
	}
	response = call(t, s, "POST", "/style/reference", string(voiceJSON(map[string]any{"asset_id": asset.ID})))
	if response.Code == 200 || calls != 1 {
		t.Fatal("foreign project reference reached inference")
	}
}

func TestReferenceVideoSampling(t *testing.T) {
	binary, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("FFmpeg required")
	}
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if raw, e := exec.Command(binary, "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=32x32:d=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", path).CombinedOutput(); e != nil {
		t.Fatal(e, string(raw))
	}
	data, _ := os.ReadFile(path)
	frames, kind, e := referenceImages(context.Background(), data)
	if e != nil || kind != "sampled_video" || len(frames) != 1 || !strings.HasPrefix(frames[0], "data:image/jpeg;base64,") {
		t.Fatal(kind, len(frames), e)
	}
	if _, _, e = referenceImages(context.Background(), []byte("not media")); e == nil {
		t.Fatal("invalid input accepted")
	}
}
