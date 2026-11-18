package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"videoflow-go/internal/providers"
)

func TestNativeCodexImageSavesAssetWithoutImageAPI(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("native success contacted API: %s", r.URL.Path)
		w.WriteHeader(500)
	})
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	session, _ := s.database.CreateDirectorSession(project, "codex", "Native images")
	ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, SessionID: session.ID})
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("OPENAI_API_KEY", "test-key-must-not-reach-native-image-turn")
	root := filepath.Join(home, "generated_images")
	os.MkdirAll(root, 0700)
	path := filepath.Join(root, "new.png")
	cli := filepath.Join(t.TempDir(), "codex")
	var buffer bytes.Buffer
	png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	encoded := base64.StdEncoding.EncodeToString(buffer.Bytes())
	script := `#!/usr/bin/env python3
import sys,json,base64
if sys.argv[1:]==['features','list']:
 print('image_generation stable true');sys.exit(0)
p=sys.stdin.read()
assert 'built-in image_gen' in p
assert '--image' in sys.argv
assert 'OPENAI_API_KEY' not in __import__('os').environ
open(` + fmt.Sprintf("%q", path) + `, 'wb').write(base64.b64decode('` + encoded + `'))
print(json.dumps({'type':'thread.started','thread_id':'isolated-image'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps({'status':'generated','path':` + fmt.Sprintf("%q", "/generated_images/new.png") + `})}}))
print(json.dumps({'type':'turn.completed'}))
`
	os.WriteFile(cli, []byte(script), 0700)
	s.directorRunner.Codex = cli
	assets, err := s.generateAssets(guidedStoryContext(ctx, project, nil), &project, nil, "", "Collar", "prop", "Red collar", "1:1", []string{"data:image/png;base64,iVBORw0KGgo="})
	if err != nil || len(assets) != 1 {
		t.Fatal(assets, err)
	}
	var metadata map[string]any
	json.Unmarshal(assets[0].MetadataJSON, &metadata)
	if metadata["provider"] != "codex" || metadata["consistency_review"] != "ready" {
		t.Fatal(metadata)
	}
	if _, err = s.imageURL(&assets[0]); err != nil {
		t.Fatal("asset was not imported into storage", err)
	}
	session, _ = s.directorSession(guidedStoryContext(ctx, project, nil))
	if session.NativeID != "" {
		t.Fatal("isolated image turn replaced planning conversation")
	}
}

func TestNativeImageRejectsArbitraryAndExistingFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new.png")
	os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	for _, candidate := range []string{filepath.Join(root, "..", "private.png"), path} {
		if _, err := readNativeImage(root, candidate, map[string]bool{path: true}); err == nil {
			t.Fatal("unsafe or stale native image imported", candidate)
		}
	}

}

func TestCodexImageFallbackOnlyForUnsupported(t *testing.T) {
	for _, outcome := range []string{"unsupported", "failed", "protocol_failure", "reference_limit"} {
		t.Run(outcome, func(t *testing.T) {
			images := 0
			s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/images/models":
					w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"input_references":{"type":"array"},"aspect_ratio":{"type":"enum","values":["1:1"]}}}]}`))
				case "/images":
					images++
					w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
				default:
					t.Errorf("unexpected API request %s", r.URL.Path)
					w.WriteHeader(500)
				}
			})
			s.cfg.UseLocalText = true
			project, _ := s.database.GetActiveProjectID()
			session, _ := s.database.CreateDirectorSession(project, "codex", "Fallback test")
			ctx := context.WithValue(context.Background(), directorBindingKey{}, directorBinding{ProjectID: project, SessionID: session.ID})
			t.Setenv("CODEX_HOME", t.TempDir())
			cli := filepath.Join(t.TempDir(), "codex")
			script := `#!/usr/bin/env python3
import sys,json
if sys.argv[1:]==['features','list']:
 print('image_generation stable true');sys.exit(0)
p=sys.stdin.read()
print(json.dumps({'type':'thread.started','thread_id':'native-fixture'}))
if 'built-in image_gen' in p:
 status=` + fmt.Sprintf("%q", outcome) + `
 if status=='protocol_failure':
  print(json.dumps({'type':'turn.failed'}));sys.exit(0)
 result={'status':status,'reason':'Fixture unsupported input' if status=='unsupported' else 'Fixture quota failure'}
else:result={'prompt':'A red collar'}
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps(result)}}))
print(json.dumps({'type':'turn.completed'}))
`
			os.WriteFile(cli, []byte(script), 0700)
			s.directorRunner.Codex = cli
			refs := []string{}
			if outcome == "reference_limit" {
				for i := 0; i < 6; i++ {
					refs = append(refs, "data:image/png;base64,iVBORw0KGgo=")
				}
			}
			assets, err := s.generateAssets(guidedStoryContext(ctx, project, nil), &project, nil, "", "Collar", "prop", "Red collar", "1:1", refs)
			if outcome == "reference_limit" {
				if err != nil || len(assets) != 1 || images != 1 {
					t.Fatal("explicit unsupported fallback failed", err, images)
				}
				var meta map[string]any
				json.Unmarshal(assets[0].MetadataJSON, &meta)
				if meta["fallback_reason"] == "" || meta["provider"] == "codex" {
					t.Fatal(meta)
				}
			} else if err == nil || images != 0 || len(assets) != 0 {
				t.Fatal("native failure triggered image API", err, images)
			}
		})
	}
}

func TestNativeImageRejectsTruncatedRaster(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.png")
	os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600)
	if _, err := readNativeImage(root, path, map[string]bool{}); err == nil {
		t.Fatal("truncated raster imported")
	}
}

func TestCodexFirstPreflightDoesNotRequireImageAPI(t *testing.T) {
	s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/models" {
			w.Write([]byte(`{"data":[{"id":"google/veo-3.1-lite","supported_durations":[1,4],"supported_aspect_ratios":["16:9"],"supported_frame_images":["first_frame"]}]}`))
			return
		}
		t.Errorf("native preflight contacted image provider: %s", r.URL.Path)
		w.WriteHeader(500)
	})
	s.cfg.UseLocalText = true
	project, _ := s.database.GetActiveProjectID()
	s.database.CreateDirectorSession(project, "codex", "Native preflight")
	cli := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(cli, []byte("#!/bin/sh\nprintf 'image_generation stable true\\n'\n"), 0700)
	s.directorRunner.Codex = cli
	response := call(t, s, "GET", "/videos/preflight?aspect_ratio=16:9&create_characters=true&scene_count=1&target_duration=1", "")
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	var result struct {
		Ready  bool `json:"ready"`
		Native bool `json:"native_images"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	if !result.Ready || !result.Native {
		t.Fatal(response.Body.String())
	}
}

func TestNativeImageVirtualPathResolvesOnlyFreshContainedRaster(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "turn")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, "result.png")
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	alias := "/generated_images/turn/result.png"
	got, err := readNativeImage(root, alias, map[string]bool{})
	if err != nil || !bytes.Equal(got.Data, buffer.Bytes()) {
		t.Fatalf("virtual image not imported: %v", err)
	}
	for _, candidate := range []string{alias, "/generated_images/../private.png", "/tmp/generated_images/turn/result.png"} {
		if _, err := readNativeImage(root, candidate, map[string]bool{path: true}); err == nil {
			t.Fatalf("unsafe or old image accepted: %s", candidate)
		}
	}
	link := filepath.Join(folder, "link.png")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readNativeImage(root, "/generated_images/turn/link.png", map[string]bool{}); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestConfiguredOpenAIImageFallbackRoutes(t *testing.T) {
	s := routerServer(t, nil)
	video, videoModel := s.routes().MediaSelection("video")
	response := call(t, s, "PUT", "/settings/providers/openai", `{"api_key":"fixture-openai"}`)
	if response.Code != 200 {
		t.Fatalf("saving OpenAI key: %d %s", response.Code, response.Body.String())
	}
	for _, kind := range []string{"image", "reference"} {
		_, provider, model, err := s.selectedMedia(context.Background(), kind)
		if err != nil || provider != "openai" || model != providers.OpenAIImageModel {
			t.Fatalf("%s route: %s %s %v", kind, provider, model, err)
		}
	}
	if p, m := s.routes().MediaSelection("video"); p != video || m != videoModel {
		t.Fatal("image credentials changed video route")
	}
}
