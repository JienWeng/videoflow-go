package api

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"videoflow-go/internal/composition"
	"videoflow-go/internal/models"
)

var compositionSlots = make(chan struct{}, 1)
var compositionImportMu sync.Mutex

func (s *Server) registerCompositionRoutes() {
	s.router.Post("/compositions/captions/import", s.handleCompositionCaptionImport)
	s.router.Get("/compositions/sources", s.handleCompositionSources)
	s.router.Get("/compositions/runtime", s.handleCompositionRuntime)
	s.router.Get("/compositions", s.handleCompositionList)
	s.router.Post("/compositions", s.handleCompositionSave)
	s.router.Get("/compositions/{revision}", s.handleCompositionGet)
	s.router.Post("/compositions/{revision}/run", s.handleCompositionRun)
}
func (s *Server) compositionRoot() string { return filepath.Join(s.cfg.StorageRoot, "compositions") }
func (s *Server) handleCompositionRuntime(w http.ResponseWriter, r *http.Request) {
	_, e := composition.Runtime()
	message := "Local HyperFrames ready"
	if e != nil {
		message = e.Error()
	}
	writeJSON(w, 200, map[string]interface{}{"ready": e == nil, "version": composition.RuntimeVersion, "source_commit": composition.SourceCommit, "message": message, "opencut": "Not integrated: adapter round-trip validation required"})
}

// handleCompositionList exposes only the active project's revision metadata.
func (s *Server) handleCompositionList(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil || !composition.ValidID(project) {
		http.Error(w, "no active project", 400)
		return
	}
	type entry struct {
		Revision     string    `json:"revision"`
		CreatedAt    time.Time `json:"created_at"`
		Duration     float64   `json:"duration"`
		ClipCount    int       `json:"clip_count"`
		CaptionCount int       `json:"caption_count"`
	}
	result := []entry{}
	directory := filepath.Join(s.compositionRoot(), project)
	files, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		writeJSON(w, 200, result)
		return
	}
	if err != nil {
		http.Error(w, "could not list saved edits", 500)
		return
	}
	for _, file := range files {
		if !file.IsDir() || !composition.ValidID(file.Name()) || file.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(directory, file.Name(), "manifest.json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		manifest, err := composition.Load(s.compositionRoot(), project, file.Name())
		if err != nil {
			continue
		}
		result = append(result, entry{file.Name(), info.ModTime().UTC(), manifest.Duration, len(manifest.Clips), len(manifest.Captions)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].Revision < result[j].Revision
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	writeJSON(w, 200, result)
}

func (s *Server) handleCompositionSave(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var m composition.Manifest
	if e = json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&m); e != nil {
		http.Error(w, "invalid manifest", 400)
		return
	}
	if e = s.validateCompositionCaptions(project, m); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	sources, _, e := s.compositionSources(project)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	for _, clips := range [][]composition.Clip{m.Clips, m.Audio} {
		for i := range clips {
			c := &clips[i]
			path, ok := sources[c.AssetID]
			if !ok {
				http.Error(w, "source asset not in active project", 400)
				return
			}
			duration, width, height, audio, err := composition.Probe(r.Context(), path)
			if err != nil {
				http.Error(w, err.Error(), 422)
				return
			}
			c.Width = width
			c.Height = height
			c.HasAudio = audio
			if duration > 0 {
				c.OriginalDuration = duration
			} else {
				c.OriginalDuration = c.Duration + c.Trim
			}
		}
	}
	revision, e := composition.Save(s.compositionRoot(), project, m, sources)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	saved, _ := composition.Load(s.compositionRoot(), project, revision)
	writeJSON(w, 201, saved)
}
func (s *Server) handleCompositionGet(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, "no active project", 400)
		return
	}
	m, e := composition.Load(s.compositionRoot(), project, chi.URLParam(r, "revision"))
	if e != nil {
		http.Error(w, "composition not found in active project", 404)
		return
	}
	writeJSON(w, 200, m)
}
func (s *Server) handleCompositionRun(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, "no active project", 400)
		return
	}
	revision := chi.URLParam(r, "revision")
	m, e := composition.Load(s.compositionRoot(), project, revision)
	if e != nil {
		http.Error(w, "composition not found in active project", 404)
		return
	}
	var body struct {
		Operation string `json:"operation"`
		Revision  string `json:"revision"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Revision != m.Revision {
		http.Error(w, "exact source revision required", 409)
		return
	}
	select {
	case compositionSlots <- struct{}{}:
		defer func() { <-compositionSlots }()
	default:
		http.Error(w, "another local composition operation is running", 409)
		return
	}
	dir := filepath.Join(s.compositionRoot(), project, revision)
	if e = composition.Verify(dir, m); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	if e = s.validateCompositionCaptions(project, m); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	log, e := composition.Run(r.Context(), dir, body.Operation)
	if e != nil {
		writeJSON(w, 422, map[string]string{"error": e.Error(), "log": log})
		return
	}
	result := map[string]interface{}{"revision": revision, "operation": body.Operation, "log": log}
	if body.Operation == "snapshot" {
		files, _ := filepath.Glob(filepath.Join(dir, "snapshots", "*.png"))
		urls := []string{}
		for _, file := range files {
			urls = append(urls, "/storage/compositions/"+project+"/"+revision+"/snapshots/"+filepath.Base(file))
		}
		result["snapshots"] = urls
	}
	if body.Operation == "render" {
		compositionImportMu.Lock()
		defer compositionImportMu.Unlock()
		p := filepath.Join(dir, "render.mp4")
		if stat, err := os.Stat(p); err != nil || stat.Size() == 0 {
			http.Error(w, "render produced no output", 500)
			return
		}
		derivedID := "asset_" + uuid.NewString()[:8]
		derivedPath := filepath.Join(s.cfg.StorageRoot, "assets", derivedID+".mp4")
		if e = os.MkdirAll(filepath.Dir(derivedPath), 0755); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			http.Error(w, readErr.Error(), 500)
			return
		}
		if e = os.WriteFile(derivedPath, data, 0600); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		meta, _ := json.Marshal(map[string]interface{}{"composition_revision": revision, "source_manifest": filepath.Join(dir, "manifest.json"), "hyperframes_version": composition.RuntimeVersion, "source_assets": append(m.Clips, m.Audio...), "caption_bindings": m.CaptionBindings})
		a := &models.Asset{ID: derivedID, ProjectID: &project, Type: "output_video", Name: "Composition " + revision[:12] + ".mp4", FilePath: derivedPath, TagsJSON: []byte("[]"), MetadataJSON: meta}
		if e = s.database.CreateUploadedAsset(a); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		result["asset"] = a
		result["asset_url"] = "/storage/assets/" + derivedID + ".mp4"
	}
	writeJSON(w, 200, result)
}

// compositionSources only resolves database-owned sources through the shared
// storage resolver; client-supplied paths never enter the source map.
type compositionSource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	MediaKind string `json:"media_kind"`
}

func (s *Server) compositionSources(project string) (map[string]string, []compositionSource, error) {
	paths := map[string]string{}
	items := []compositionSource{}
	add := func(id, name, kind, path string) {
		full, e := s.storagePath(path)
		if e != nil {
			return
		}
		info, e := os.Lstat(full)
		if e != nil || !info.Mode().IsRegular() {
			return
		}
		paths[id] = full
		mediaKind := "unknown"
		switch strings.ToLower(filepath.Ext(full)) {
		case ".png", ".jpg", ".jpeg", ".webp", ".gif":
			mediaKind = "image"
		case ".mp4", ".webm", ".mov", ".mkv":
			mediaKind = "video"
		case ".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus":
			mediaKind = "audio"
		}
		items = append(items, compositionSource{ID: id, Name: name, Type: kind, MediaKind: mediaKind})
	}
	assets, e := s.database.ListAssets(project)
	if e != nil {
		return nil, nil, e
	}
	for _, a := range assets {
		if a.ProjectID != nil && *a.ProjectID == project {
			add(a.ID, a.Name, a.Type, a.FilePath)
		}
	}
	jobs, e := s.database.ListRenderJobs(project)
	if e != nil {
		return nil, nil, e
	}
	for _, job := range jobs {
		if job.ProjectID == nil || *job.ProjectID != project || job.Status != "succeeded" {
			continue
		}
		outputs, e := s.database.ListOutputsForJob(job.ID)
		if e != nil {
			return nil, nil, e
		}
		for _, out := range outputs {
			add("output-"+out.ID, "Rendered output "+out.ID, "output_video", out.VideoPath)
		}
	}
	return paths, items, nil
}
func (s *Server) handleCompositionSources(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, "no active project", 400)
		return
	}
	_, items, e := s.compositionSources(project)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 200, items)
}
