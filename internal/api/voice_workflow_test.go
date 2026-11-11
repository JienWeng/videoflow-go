package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"videoflow-go/internal/models"
)

func voiceFixture(t *testing.T, handler http.HandlerFunc) *Server {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	remote := httptest.NewServer(handler)
	t.Cleanup(remote.Close)
	s.cfg.FalAPIKey = "test"
	s.cfg.FalBaseURL = remote.URL
	if err := s.database.EnsureVoiceWorkflow(); err != nil {
		t.Fatal(err)
	}
	return s
}
func voiceCharacter(t *testing.T, s *Server) string {
	project, _ := s.database.GetActiveProjectID()
	c := &models.Character{ProjectID: &project, Name: "Speaker"}
	if err := s.database.CreateCharacter(c); err != nil {
		t.Fatal(err)
	}
	return c.ID
}
func TestVoiceRunDurableIDAndDuplicateFence(t *testing.T) {
	var submits atomic.Int32
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			submits.Add(1)
			w.Write([]byte(`{"request_id":"durable-id"}`))
			return
		}
		w.Write([]byte(`{"status":"IN_QUEUE"}`))
	})
	c := voiceCharacter(t, s)
	profile := call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"mode":"saved","voice_id":"approved-custom-id","approved":true}}`)
	if profile.Code != 200 {
		t.Fatal(profile.Code, profile.Body.String())
	}
	script := fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u1","character_id":%q,"language":"en","text":"Exact approved words."}]}}`, c)
	if r := call(t, s, "PUT", "/voice/scripts/main", script); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	prepared := call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"speech","script_id":"main","line_id":"u1"}`)
	if prepared.Code != 200 {
		t.Fatal(prepared.Code, prepared.Body.String())
	}
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(prepared.Body.Bytes(), &task)
	run := fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint)
	first := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", run)
	if first.Code != 200 {
		t.Fatal(first.Code, first.Body.String())
	}
	if !json.Valid(first.Body.Bytes()) || !strings.Contains(first.Body.String(), "durable-id") {
		t.Fatal(first.Body.String())
	}
	second := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", run)
	if second.Code != 409 || submits.Load() != 1 {
		t.Fatal(second.Code, submits.Load())
	}
	project, _ := s.database.GetActiveProjectID()
	saved, _ := s.database.GetVoiceTask(project, task.ID)
	if saved.UpstreamID != "durable-id" || saved.Status != "queued" {
		t.Fatalf("%+v", saved)
	}
}
func TestVoiceStaleApprovalAndCrossProjectSampleFailBeforePayment(t *testing.T) {
	var paid atomic.Int32
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) { paid.Add(1) })
	c := voiceCharacter(t, s)
	r := call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"voice_id":"saved","approved":true}}`)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	p, _ := s.database.CreateProject("Other", "")
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "sample.wav"), []byte("sample"), 0600)
	a := &models.Asset{ProjectID: &p.ID, Type: "audio", Name: "sample", FilePath: "storage/sample.wav"}
	s.database.CreateAsset(a)
	r = call(t, s, "PUT", "/voice/profiles/"+c, fmt.Sprintf(`{"revision":1,"data":{"sample_asset_id":%q,"approved":true}}`, a.ID))
	if r.Code < 400 {
		t.Fatal("accepted cross-project audio")
	}
	script := fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Hello."}]}}`, c)
	call(t, s, "PUT", "/voice/scripts/main", script)
	prepared := call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"speech","script_id":"main","line_id":"u"}`)
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(prepared.Body.Bytes(), &task)
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":1,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Changed."}]}}`, c))
	run := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	if run.Code != 409 || paid.Load() != 0 {
		t.Fatal(run.Code, run.Body.String(), paid.Load())
	}
	_ = project
}
func TestVoiceUncertainSubmissionOnlyReconcilesNeverResubmits(t *testing.T) {
	var submissions atomic.Int32
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			submissions.Add(1)
			w.WriteHeader(502)
			return
		}
		w.Write([]byte(`{"status":"IN_QUEUE"}`))
	})
	c := voiceCharacter(t, s)
	call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"voice_id":"saved","approved":true}}`)
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Hello."}]}}`, c))
	prepared := call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"speech","script_id":"main","line_id":"u"}`)
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(prepared.Body.Bytes(), &task)
	run := fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint)
	first := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", run)
	if first.Code != 200 || !strings.Contains(first.Body.String(), "uncertain") {
		t.Fatal(first.Code, first.Body.String())
	}
	if second := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", run); second.Code != 409 {
		t.Fatal(second.Body.String())
	}
	rec := call(t, s, "POST", "/voice/tasks/"+task.ID+"/reconcile", `{"upstream_id":"recovered","reviewed":true}`)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	refresh := call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`)
	if refresh.Code != 200 || submissions.Load() != 1 {
		t.Fatal(refresh.Code, refresh.Body.String(), submissions.Load())
	}
}

func voiceWAV(seconds int) []byte {
	const rate = 8000
	var b bytes.Buffer
	size := seconds * rate * 2
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+size))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint32(rate))
	binary.Write(&b, binary.LittleEndian, uint32(rate*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(size))
	b.Write(make([]byte, size))
	return b.Bytes()
}
func uploadVoiceFixture(t *testing.T, s *Server, character string, seconds int) *models.Asset {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "voice.wav")
	if err != nil {
		t.Fatal(err)
	}
	file.Write(voiceWAV(seconds))
	form.WriteField("type", "audio")
	form.WriteField("character_id", character)
	form.Close()
	req := httptest.NewRequest("POST", "/assets/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var asset models.Asset
	if err = json.Unmarshal(rec.Body.Bytes(), &asset); err != nil {
		t.Fatal(err)
	}
	characterDoc, err := s.database.GetCharacter(character)
	if err != nil {
		t.Fatal(err)
	}
	var visualIDs []string
	json.Unmarshal(characterDoc.ReferenceAssetIDsJSON, &visualIDs)
	for _, id := range visualIDs {
		if id == asset.ID {
			t.Fatal("voice audio sample polluted character visual reference IDs")
		}
	}
	return &asset
}
func TestVoiceActualSampleUploadCloneAndNativeReferenceCompatibility(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("FFprobe fixture")
	}
	var submits atomic.Int32
	var cloneAudio, nativeAudio bool
	s := voiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			submits.Add(1)
			var input map[string]any
			json.NewDecoder(r.Body).Decode(&input)
			if strings.HasSuffix(r.URL.Path, "voice-clone") {
				cloneAudio = strings.HasPrefix(input["audio_url"].(string), "data:audio/wav;base64,")
			} else if strings.HasSuffix(r.URL.Path, "reference-to-video") {
				audio := input["audio_urls"].([]any)
				nativeAudio = len(audio) == 1 && strings.HasPrefix(audio[0].(string), "data:audio/wav;base64,")
				if _, ok := input["image_url"]; ok {
					t.Error("strict first frame leaked into reference mode")
				}
			}
			w.Write([]byte(`{"request_id":"receipt"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.Write([]byte(`{"status":"COMPLETED"}`))
			return
		}
		w.Write([]byte(`{"custom_voice_id":"cloned-speaker"}`))
	})
	c := voiceCharacter(t, s)
	sample := uploadVoiceFixture(t, s, c, 10)
	profile := fmt.Sprintf(`{"revision":0,"data":{"mode":"saved","sample_asset_id":%q,"approved":true}}`, sample.ID)
	if r := call(t, s, "PUT", "/voice/profiles/"+c, profile); r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	prepared := call(t, s, "POST", "/voice/tasks/prepare", fmt.Sprintf(`{"kind":"clone","character_id":%q}`, c))
	if prepared.Code != 200 {
		t.Fatal(prepared.Code, prepared.Body.String())
	}
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(prepared.Body.Bytes(), &task)
	if r := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint)); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if r := call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`); r.Code != 200 || !strings.Contains(r.Body.String(), "review_required") {
		t.Fatal(r.Code, r.Body.String())
	}
	if r := call(t, s, "POST", "/voice/tasks/"+task.ID+"/accept-voice", `{"reviewed":true}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	project, _ := s.database.GetActiveProjectID()
	doc, _ := s.database.GetVoiceDocument(project, "profile", c)
	if !strings.Contains(string(doc.Data), "cloned-speaker") {
		t.Fatal(string(doc.Data))
	}
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Hello."}]}}`, c))
	// Explicit native mode uses a real audio sample and separate compatible route.
	r := call(t, s, "PUT", "/voice/profiles/"+c, fmt.Sprintf(`{"revision":2,"data":{"mode":"native_reference","sample_asset_id":%q,"approved":true}}`, sample.ID))
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	image := &models.Asset{ID: "visual", ProjectID: &project, Type: "character_reference", Name: "visual", FilePath: "storage/image.png"}
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "image.png"), []byte("\x89PNG\r\n\x1a\n"), 0600)
	s.database.CreateAsset(image)
	prepared = call(t, s, "POST", "/voice/tasks/prepare", fmt.Sprintf(`{"kind":"native_reference","script_id":"main","line_id":"u","image_asset_ids":[%q],"prompt":"Speaker @Image1 says hello","duration":5}`, image.ID))
	if prepared.Code != 200 {
		t.Fatal(prepared.Code, prepared.Body.String())
	}
	json.Unmarshal(prepared.Body.Bytes(), &task)
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	if r.Code != 200 || !cloneAudio || !nativeAudio || submits.Load() != 2 {
		t.Fatal(r.Code, r.Body.String(), cloneAudio, nativeAudio, submits.Load())
	}
	// A first/last-frame request cannot sneak through this explicit mode.
	r = call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"native_reference","image_url":"strict-frame"}`)
	if r.Code != 400 {
		t.Fatal("accepted unsupported strict frame mode")
	}
}
func voiceMediaFixtures(t *testing.T) ([]byte, []byte) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg fixture required")
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	audio := filepath.Join(dir, "speech.mp3")
	if raw, err := exec.Command("ffmpeg", "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=160x120:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", clip).CombinedOutput(); err != nil {
		t.Fatalf("video fixture %v %s", err, raw)
	}
	if raw, err := exec.Command("ffmpeg", "-v", "error", "-y", "-i", clip, "-vn", audio).CombinedOutput(); err != nil {
		t.Fatalf("audio fixture %v %s", err, raw)
	}
	v, _ := os.ReadFile(clip)
	a, _ := os.ReadFile(audio)
	return v, a
}
func voiceOutputFixture(t *testing.T, s *Server, clip []byte) *models.RenderOutput {
	t.Helper()
	project, _ := s.database.GetActiveProjectID()
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "final.mp4"), clip, 0600)
	job := &models.RenderJob{ProjectID: &project, Provider: "local", Model: "fixture", Status: "succeeded"}
	out := &models.RenderOutput{VideoPath: "storage/final.mp4"}
	if err := s.database.SaveLocalOutput(job, out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestScriptCaptionFinalAudioAlignmentApplyExportAndStaleness(t *testing.T) {
	clip, _ := voiceMediaFixtures(t)
	var submits atomic.Int32
	var s *Server
	s = voiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			submits.Add(1)
			var input map[string]any
			json.NewDecoder(r.Body).Decode(&input)
			if input["text"] != "Hello, world!" || !strings.HasPrefix(input["audio_url"].(string), "data:audio/mpeg;base64,") {
				t.Error("alignment did not receive approved words and final audio")
			}
			w.Write([]byte(`{"request_id":"alignment-receipt"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.Write([]byte(`{"status":"COMPLETED"}`))
			return
		}
		w.Write([]byte(`{"words":[{"text":"Hello,","start":0.1,"end":0.6},{"text":"world!","start":0.7,"end":1.8}],"loss":0.1}`))
	})
	c := voiceCharacter(t, s)
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Hello, world!"}]}}`, c))
	out := voiceOutputFixture(t, s, clip)
	prepared := call(t, s, "POST", "/voice/tasks/prepare", fmt.Sprintf(`{"kind":"alignment","script_id":"main","output_id":%q}`, out.ID))
	if prepared.Code != 200 {
		t.Fatal(prepared.Code, prepared.Body.String())
	}
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(prepared.Body.Bytes(), &task)
	if strings.Contains(prepared.Body.String(), "base64") {
		t.Fatal("raw audio leaked in review response")
	}
	r := call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), "review_required") {
		t.Fatal(r.Code, r.Body.String())
	}
	before, _ := s.database.GetRenderOutput(out.ID)
	if len(before.CaptionsJSON) > 2 {
		t.Fatal("alignment silently replaced captions without review")
	}
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/apply-captions", `{"reviewed":true}`)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	for _, format := range []string{"srt", "vtt", "json"} {
		r = call(t, s, "GET", "/voice/outputs/"+out.ID+"/captions/export?format="+format, "")
		if r.Code != 200 || !strings.Contains(r.Body.String(), "Hello, world!") {
			t.Fatal(format, r.Code, r.Body.String())
		}
	}
	saved, _ := s.database.GetRenderOutput(out.ID)
	if err := s.ValidateScriptCaptionSource(saved); err != nil {
		t.Fatal(err)
	}
	// Retiming/editing keeps the original source binding, so a later source edit
	// cannot erase the stale fence by passing through the existing caption editor.
	saved.CaptionsJSON = s.ScriptCaptionPayload(saved, []models.CaptionSegment{{ID: "u", CharacterID: c, Language: "en", Start: .2, End: 1.7, Text: "Hello, world!"}}, "clean")
	s.database.UpdateRenderOutput(saved)
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "final.mp4"), append(append([]byte{}, clip...), 1), 0600)
	if err := s.ValidateScriptCaptionSource(saved); err == nil {
		t.Fatal("final audio/output replacement did not invalidate alignment")
	}
	os.WriteFile(filepath.Join(s.cfg.StorageRoot, "final.mp4"), clip, 0600)
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":1,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Changed wording."}]}}`, c))
	r = call(t, s, "GET", "/voice/outputs/"+out.ID+"/captions/export?format=srt", "")
	if r.Code != 409 {
		t.Fatal("export accepted stale wording", r.Code, r.Body.String())
	}
	if err := s.ValidateScriptCaptionSource(saved); err == nil {
		t.Fatal("caption source fence lost through editing")
	}
	if submits.Load() != 1 {
		t.Fatal(submits.Load())
	}
}
func TestSavedSpeechImportsLocalAudioAndLipSyncUsesMatchingLine(t *testing.T) {
	clip, audio := voiceMediaFixtures(t)
	var s *Server
	var submits atomic.Int32
	s = voiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			submits.Add(1)
			if strings.Contains(r.URL.Path, "sync-lipsync") {
				w.Write([]byte(`{"request_id":"lip-receipt"}`))
			} else {
				w.Write([]byte(`{"request_id":"speech-receipt"}`))
			}
			return
		}
		if r.URL.Path == "/audio.mp3" {
			w.Write(audio)
			return
		}
		if r.URL.Path == "/video.mp4" {
			w.Write(clip)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/status") {
			w.Write([]byte(`{"status":"COMPLETED"}`))
			return
		}
		if strings.Contains(r.URL.Path, "sync-lipsync") {
			fmt.Fprintf(w, `{"video":{"url":%q}}`, s.cfg.FalBaseURL+"/video.mp4")
		} else {
			fmt.Fprintf(w, `{"audio":{"url":%q}}`, s.cfg.FalBaseURL+"/audio.mp3")
		}
	})
	c := voiceCharacter(t, s)
	call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":0,"data":{"voice_id":"custom-speaker","approved":true}}`)
	call(t, s, "PUT", "/voice/scripts/main", fmt.Sprintf(`{"revision":0,"data":{"approved":true,"lines":[{"id":"u","character_id":%q,"language":"en","text":"Hello!","visible":true}]}}`, c))
	prepared := call(t, s, "POST", "/voice/tasks/prepare", `{"kind":"speech","script_id":"main","line_id":"u"}`)
	var task struct{ ID, Fingerprint string }
	json.Unmarshal(prepared.Body.Bytes(), &task)
	call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	r := call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"status":"completed"`) {
		t.Fatal(r.Code, r.Body.String())
	}
	var response struct {
		Result struct {
			AssetID string `json:"asset_id"`
		}
	}
	json.Unmarshal(r.Body.Bytes(), &response)
	asset, err := s.database.GetAsset(response.Result.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if asset.CharacterID == nil || *asset.CharacterID != c || !strings.Contains(string(asset.MetadataJSON), `"lip_sync_required":true`) {
		t.Fatal(asset)
	}
	output := voiceOutputFixture(t, s, clip)
	prepared = call(t, s, "POST", "/voice/tasks/prepare", fmt.Sprintf(`{"kind":"lipsync","script_id":"main","line_id":"u","audio_asset_id":%q,"output_id":%q}`, asset.ID, output.ID))
	if prepared.Code != 200 {
		t.Fatal(prepared.Code, prepared.Body.String())
	}
	json.Unmarshal(prepared.Body.Bytes(), &task)
	call(t, s, "POST", "/voice/tasks/"+task.ID+"/run", fmt.Sprintf(`{"confirmed":true,"fingerprint":%q}`, task.Fingerprint))
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"status":"completed"`) || !strings.Contains(r.Body.String(), "output_id") {
		t.Fatal(r.Code, r.Body.String())
	}
	// Retrieval is idempotent even when refreshed again after import.
	r = call(t, s, "POST", "/voice/tasks/"+task.ID+"/refresh", `{}`)
	if r.Code != 200 || submits.Load() != 2 {
		t.Fatal(r.Code, submits.Load())
	}
	call(t, s, "PUT", "/voice/profiles/"+c, `{"revision":1,"data":{"voice_id":"changed-speaker","approved":true}}`)
	r = call(t, s, "POST", "/voice/tasks/prepare", fmt.Sprintf(`{"kind":"lipsync","script_id":"main","line_id":"u","audio_asset_id":%q,"output_id":%q}`, asset.ID, output.ID))
	if r.Code != 400 || submits.Load() != 2 {
		t.Fatal("stale saved voice audio allowed in lip sync", r.Code, r.Body.String())
	}
}
func TestVoiceResultRejectsWrongMediaStream(t *testing.T) {
	_, audio := voiceMediaFixtures(t)
	path := filepath.Join(t.TempDir(), "pretend-video.mp4")
	os.WriteFile(path, audio, 0600)
	if _, err := voiceRequireMedia(context.Background(), path, "video"); err == nil {
		t.Fatal("audio-only provider result accepted as generated video")
	}
	if _, err := voiceRequireMedia(context.Background(), path, "audio"); err != nil {
		t.Fatal(err)
	}
}
