package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"videoflow-go/internal/models"

	"github.com/google/uuid"
	"videoflow-go/internal/director"
	"videoflow-go/internal/providers"
)

func (s *Server) preferCodexImages(ctx context.Context) bool {
	if s.cfg.DisableCodexImages {
		return false
	}
	session, err := s.directorSession(ctx)
	return err == nil && session.Engine == "codex" && s.directorRunner.ImagesAvailable(ctx)
}

func codexGeneratedRoot() string {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, ".codex")
	}
	absolute, _ := filepath.Abs(filepath.Join(root, "generated_images"))
	return absolute
}

func imageFiles(root string) map[string]bool {
	result := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			result[path] = true
		}
		return nil
	})
	return result
}

func readNativeImage(root, path string, before map[string]bool) (providers.GeneratedImage, error) {
	empty := providers.GeneratedImage{}
	root, err := filepath.Abs(root)
	if err != nil {
		return empty, err
	}
	// image_gen may report its virtual /generated_images/ path rather than
	// CODEX_HOME's physical path. Resolve only that exact namespace; the
	// containment, freshness and symlink checks below remain authoritative.
	if strings.HasPrefix(path, "/generated_images/") {
		path = filepath.Join(root, strings.TrimPrefix(path, "/generated_images/"))
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return empty, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || before[path] {
		return empty, fmt.Errorf("Codex did not return a new generated image")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return empty, fmt.Errorf("Codex image path is unavailable or uses symlinks")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 64<<20 {
		return empty, fmt.Errorf("Codex image is missing or exceeds the image size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return empty, err
	}
	kind := http.DetectContentType(data)
	if kind != "image/png" && kind != "image/jpeg" && kind != "image/webp" {
		return empty, fmt.Errorf("Codex did not return a supported raster image")
	}
	config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
	if decodeErr != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return empty, fmt.Errorf("Codex image has invalid dimensions or raster data")
	}
	if _, _, decodeErr = image.Decode(bytes.NewReader(data)); decodeErr != nil {
		return empty, fmt.Errorf("Codex returned a corrupt raster image")
	}
	return providers.GeneratedImage{Data: data, MediaType: kind}, nil
}

// The bool indicates explicit unavailability/unsupported input, the only automatic fallback cases.
// Native failures/timeouts never trigger a second image generation request.
func (s *Server) codexImage(ctx context.Context, prompt, aspect string, references []string) ([]providers.GeneratedImage, bool, string, error) {
	if !s.preferCodexImages(ctx) {
		return nil, true, "Codex image generation is unavailable for the selected local engine", nil
	}
	if len(references) > 5 {
		return nil, true, "Native Codex supports at most five input references; the API receives all selected references", nil
	}
	session, err := s.directorSession(ctx)
	if err != nil {
		return nil, false, "", err
	}
	release, err := takeDirectorTurn(ctx)
	if err != nil {
		return nil, false, "", err
	}
	defer release()
	privateRoot := filepath.Join(filepath.Dir(s.cfg.DatabasePath), ".videoflow-image-workspaces")
	if err = os.MkdirAll(privateRoot, 0700); err != nil {
		return nil, false, "", err
	}
	workspace, err := os.MkdirTemp(privateRoot, "codex-image-")
	if err != nil {
		return nil, false, "", err
	}
	defer os.RemoveAll(workspace)
	paths := []string{}
	for i, reference := range references {
		header, encoded, ok := strings.Cut(reference, ",")
		if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
			return nil, true, "Native image inputs require local raster references", nil
		}
		data, e := base64.StdEncoding.DecodeString(encoded)
		if e != nil || len(data) > 64<<20 {
			return nil, false, "", fmt.Errorf("invalid image reference")
		}
		extension := ""
		switch http.DetectContentType(data) {
		case "image/png":
			extension = ".png"
		case "image/jpeg":
			extension = ".jpg"
		case "image/webp":
			extension = ".webp"
		default:
			return nil, true, "Native image inputs require PNG, JPEG or WebP references", nil
		}
		path := filepath.Join(workspace, fmt.Sprintf("reference-%d%s", i+1, extension))
		if e = os.WriteFile(path, data, 0600); e != nil {
			return nil, false, "", e
		}
		paths = append(paths, path)
	}
	root := codexGeneratedRoot()
	before := imageFiles(root)
	instruction := "Use only the built-in image_gen tool to generate exactly one production image. No shell, scripts, API keys, HTTP requests, placeholders or code drawings. Preserve all attached reference identities and locked traits. Use the attached images as references in image_gen via referenced_image_paths. Requested aspect ratio: " + aspect + ". Reference paths: " + strings.Join(paths, ", ") + "\nBrief:\n" + prompt + "\nAfter success return only JSON {\"status\":\"generated\",\"path\":\"absolute saved generated_images path\"}. If image_gen is absent or cannot support these inputs BEFORE calling it, return {\"status\":\"unsupported\",\"reason\":\"explanation\"}. For quota, authentication, timeout or generation failure return {\"status\":\"failed\",\"reason\":\"explanation\"}; do not retry or use an API."
	request, _ := json.Marshal(map[string]any{"prompt": prompt, "aspect_ratio": aspect, "reference_count": len(paths), "route": "codex_native_image"})
	turn, err := s.database.BeginDirectorTurn(session.ID, session.ProjectID, uuid.NewString(), "image_generation", request)
	if err != nil {
		return nil, false, "", err
	}
	turnCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	s.directorCancels.Store(session.ID, cancel)
	raw, runErr := s.directorRunner.Run(turnCtx, director.Request{Engine: "codex", Workspace: workspace, Prompt: instruction, ImageGeneration: true, Images: paths}, func(string) error { return nil })
	interrupted := turnCtx.Err() != nil
	cancel()
	s.directorCancels.Delete(session.ID)
	var result struct {
		Status string `json:"status"`
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}
	if runErr == nil {
		runErr = json.Unmarshal(raw, &result)
	}
	if runErr == nil {
		snapshot, _ := ctx.Value(directorSnapshotKey{}).(*directorContext)
		binding, _ := ctx.Value(directorBindingKey{}).(directorBinding)
		if snapshot != nil {
			current, e := s.buildDirectorContext(binding.ProjectID, binding.Selection)
			if e != nil || current.Revision != snapshot.Revision {
				runErr = errDirectorContextChanged
			}
		}
	}
	var images []providers.GeneratedImage
	if runErr == nil {
		switch result.Status {
		case "generated":
			var image providers.GeneratedImage
			image, runErr = readNativeImage(root, result.Path, before)
			if runErr == nil {
				images = []providers.GeneratedImage{image}
			}
		case "unsupported":
			runErr = fmt.Errorf("Codex reported unsupported after starting its image turn; API fallback was not started because generation may have been attempted")
		default:
			runErr = fmt.Errorf("Codex image generation did not complete; check the native image turn before retrying")
		}
	}
	status, message := models.StatusSucceeded, ""
	if runErr != nil {
		status = models.StatusFailed
		message = runErr.Error()
		if interrupted {
			status = "interrupted"
		}
	}
	if e := s.database.FinishDirectorTurn(turn.ID, status, raw, message); e != nil {
		return nil, false, "", e
	}
	if runErr != nil {
		return nil, false, "", runErr
	}
	return images, false, result.Reason, nil
}
