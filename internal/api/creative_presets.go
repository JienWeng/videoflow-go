package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"net/http"
	"strings"
	"videoflow-go/internal/presets"
)

const packsKey = "creative_preset_packs_v1"

func (s *Server) presetPacks() ([]presets.Pack, string, error) {
	raw, err := s.database.GetAppSetting(packsKey)
	if err == sql.ErrNoRows {
		return []presets.Pack{}, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var packs []presets.Pack
	if err = json.Unmarshal([]byte(raw), &packs); err != nil {
		return nil, "", fmt.Errorf("saved preset packs could not be read")
	}
	return packs, raw, nil
}
func packHash(p presets.Pack) string {
	b, _ := json.Marshal(p)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (s *Server) findPreset(id string) (*presets.Preset, error) {
	for _, p := range presets.Builtins().Presets {
		if p.ID == id {
			return &p, nil
		}
	}
	packs, _, err := s.presetPacks()
	if err != nil {
		return nil, err
	}
	for _, pack := range packs {
		for _, p := range pack.Presets {
			if "community:"+pack.ID+":"+p.ID == id {
				p.ID = id
				return &p, nil
			}
		}
	}
	return nil, fmt.Errorf("preset is unavailable; choose another style or reinstall its pack")
}
func (s *Server) handlePresetCatalog(w http.ResponseWriter, r *http.Request) {
	packs, _, err := s.presetPacks()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	options := presets.Builtins().Presets
	type installed struct {
		Pack presets.Pack `json:"pack"`
		Hash string       `json:"hash"`
	}
	list := []installed{}
	for _, pack := range packs {
		list = append(list, installed{pack, packHash(pack)})
		for _, p := range pack.Presets {
			p.ID = "community:" + pack.ID + ":" + p.ID
			options = append(options, p)
		}
	}
	project, _ := s.activeProjectID(r.Context())
	_, hasStyle := s.effectiveStyle(project, "")
	entries := []map[string]any{}
	for _, p := range options {
		var entry map[string]any
		_ = json.Unmarshal(voiceJSON(p), &entry)
		entry["fingerprint"] = creativeStyleFingerprint(p)
		entries = append(entries, entry)
	}
	var projectStyle any
	if style, e := s.database.GetStyleGuide(project); e == nil && hasStyle {
		projectStyle = map[string]any{"id": "project-style", "name": style.Name, "description": style.StylePrompt, "fingerprint": creativeStyleFingerprint(style)}
	}
	writeJSON(w, 200, map[string]any{"has_project_style": hasStyle, "project_style": projectStyle, "presets": entries, "packs": list, "built_in_count": 6})
}
func (s *Server) handlePresetPack(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, presets.MaxBytes))
	if err != nil {
		http.Error(w, "pack exceeds 512 KiB", 400)
		return
	}
	pack, err := presets.Parse(raw)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	packs, old, err := s.presetPacks()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	index := -1
	hash := ""
	for i, p := range packs {
		if p.ID == pack.ID {
			index = i
			hash = packHash(p)
		}
	}
	if strings.HasSuffix(r.URL.Path, "/preview") {
		writeJSON(w, 200, map[string]any{"pack": pack, "existing_hash": hash, "unchanged": hash == packHash(pack)})
		return
	}
	if index >= 0 && hash != packHash(pack) && r.Header.Get("If-Match") != hash {
		http.Error(w, "pack already exists or changed; preview again before replacing", 409)
		return
	}
	if index >= 0 {
		packs[index] = pack
	} else {
		if len(packs) >= 10 {
			http.Error(w, "remove a pack before installing more (maximum 10)", 400)
			return
		}
		packs = append(packs, pack)
	}
	next, _ := json.Marshal(packs)
	ok, err := s.database.CompareAppSetting(packsKey, old, string(next))
	if err != nil {
		http.Error(w, "could not save preset pack", 500)
		return
	}
	if !ok {
		http.Error(w, "preset packs changed; preview again", 409)
		return
	}
	writeJSON(w, 200, map[string]any{"installed": pack.ID})
}
func (s *Server) handleDeletePresetPack(w http.ResponseWriter, r *http.Request) {
	packs, old, err := s.presetPacks()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	id := chi.URLParam(r, "id")
	index := -1
	for i, p := range packs {
		if p.ID == id {
			index = i
			if r.Header.Get("If-Match") != packHash(p) {
				http.Error(w, "pack changed; reload before removing", 409)
				return
			}
		}
	}
	if index < 0 {
		http.Error(w, "pack not found", 404)
		return
	}
	packs = append(packs[:index], packs[index+1:]...)
	next, _ := json.Marshal(packs)
	ok, err := s.database.CompareAppSetting(packsKey, old, string(next))
	if err != nil {
		http.Error(w, "could not remove pack", 500)
		return
	}
	if !ok {
		http.Error(w, "packs changed; reload", 409)
		return
	}
	writeJSON(w, 200, map[string]bool{"removed": true})
}
func (s *Server) handleExportPresetPack(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	pack := presets.Builtins()
	if id != "starter" {
		packs, _, err := s.presetPacks()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		found := false
		for _, p := range packs {
			if p.ID == id {
				pack = p
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "pack not found", 404)
			return
		}
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.videoflow-presets.json"`, pack.ID))
	writeJSON(w, 200, pack)
}
