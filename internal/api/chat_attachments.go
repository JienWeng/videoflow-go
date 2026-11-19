package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"videoflow-go/internal/models"
	"videoflow-go/internal/providers"
)

const chatAttachmentLimit = 4

// chatAttachment is one image or video the user pasted, dropped or linked into the chat.
type chatAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"` // image or video
	path string
}

func mediaKindForPath(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return "image"
	case ".mp4", ".mov", ".webm":
		return "video"
	case ".mp3", ".wav":
		return "audio"
	}
	return ""
}

// chatAttachments resolves attachment ids to this project's media. Unknown ids are an
// error, so the assistant never plans around a file it was not actually given.
func (s *Server) chatAttachments(project string, ids []string) ([]chatAttachment, error) {
	if len(ids) > chatAttachmentLimit {
		return nil, fmt.Errorf("attach at most %d files to one message", chatAttachmentLimit)
	}
	out := []chatAttachment{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		asset, err := s.database.GetAsset(id)
		if err != nil || asset.ProjectID == nil || *asset.ProjectID != project {
			return nil, fmt.Errorf("an attached file is no longer in this project; attach it again")
		}
		full, err := s.storagePath(asset.FilePath)
		kind := mediaKindForPath(asset.FilePath)
		if err != nil || (kind != "image" && kind != "video") {
			return nil, fmt.Errorf("attach images or videos (PNG, JPEG, GIF, WebP, MP4, MOV, WebM)")
		}
		out = append(out, chatAttachment{ID: asset.ID, Name: asset.Name, Kind: kind, path: full})
	}
	return out, nil
}

type directorImagesKey struct{}

// attachmentPreviews writes small JPEG copies the planner can look at: the image itself,
// or three sampled frames of a video. The caller removes the returned directory.
func attachmentPreviews(ctx context.Context, attachments []chatAttachment) (paths []string, dir string, err error) {
	if len(attachments) == 0 {
		return nil, "", nil
	}
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, "", nil // The assistant still gets names and kinds, just not the pixels.
	}
	if dir, err = os.MkdirTemp("", "videoflow-chat-*"); err != nil {
		return nil, "", err
	}
	bounded, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for i, a := range attachments {
		pattern := filepath.Join(dir, fmt.Sprintf("a%d-%%d.jpg", i))
		filter, frames := "scale='min(1280,iw)':-2", "1"
		if a.Kind == "video" {
			filter, frames = "fps=1/2:round=up,scale='min(768,iw)':-2", "3"
		}
		// Local file decoding only. A failed preview is skipped, not fatal.
		_ = exec.CommandContext(bounded, binary, "-v", "error", "-nostdin", "-threads", "2", "-protocol_whitelist", "file,pipe", "-i", a.path, "-an", "-sn", "-vf", filter, "-frames:v", frames, "-q:v", "4", pattern).Run()
		found, _ := filepath.Glob(filepath.Join(dir, fmt.Sprintf("a%d-*.jpg", i)))
		paths = append(paths, found...)
	}
	return paths, dir, nil
}

// handleImportLinkAsset saves a pasted public link (image, video, or a web page's
// preview image) into the project so it can be attached like an upload.
func (s *Server) handleImportLinkAsset(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body) != nil || !strings.HasPrefix(strings.TrimSpace(body.URL), "https://") {
		http.Error(w, "paste a public https:// link to an image, video or page", 400)
		return
	}
	link := strings.TrimSpace(body.URL)
	data, err := providers.DownloadReference(r.Context(), link, referenceLimit)
	if err != nil {
		http.Error(w, "Couldn't fetch that link ("+err.Error()+"). Save the image or clip and paste the file instead.", 422)
		return
	}
	if strings.HasPrefix(http.DetectContentType(data), "text/html") {
		preview, e := referencePreview(data, link)
		if e != nil {
			http.Error(w, "That page has no preview image to use. Save the image or clip and paste the file instead.", 422)
			return
		}
		if data, e = providers.DownloadReference(r.Context(), preview, referenceLimit); e != nil {
			http.Error(w, "Couldn't fetch that page's preview image. Save the image or clip and paste the file instead.", 422)
			return
		}
	}
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp", "video/mp4": ".mp4", "video/webm": ".webm"}[http.DetectContentType(data)]
	if ext == "" && referenceIsVideo(data) {
		ext = ".mp4"
	}
	if ext == "" {
		http.Error(w, "That link isn't an image or video this studio can use. Save the file and paste it instead.", 422)
		return
	}
	if _, _, err = referenceImages(r.Context(), data); err != nil {
		http.Error(w, err.Error(), 422)
		return
	}
	name := "Pasted link"
	if u, e := url.Parse(link); e == nil {
		if base := path.Base(u.Path); base != "" && base != "/" && base != "." && len(base) <= 80 {
			name = base
		} else {
			name = u.Hostname()
		}
	}
	dir := filepath.Join(s.cfg.StorageRoot, "assets")
	if err = os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, "failed to prepare storage", 500)
		return
	}
	id := "asset_" + uuid.New().String()[:8]
	dest := filepath.Join(dir, id+ext)
	if err = os.WriteFile(dest, data, 0644); err != nil {
		http.Error(w, "failed to save file", 500)
		return
	}
	meta, _ := json.Marshal(map[string]string{"source_url": link})
	asset := &models.Asset{ID: id, ProjectID: &project, Type: "other", Name: name, FilePath: filepath.ToSlash(filepath.Join("storage", "assets", id+ext)), TagsJSON: []byte("[]"), MetadataJSON: meta}
	if err = s.database.CreateUploadedAsset(asset); err != nil {
		os.Remove(dest)
		http.Error(w, "failed to save asset", 500)
		return
	}
	writeJSON(w, 200, asset)
}
