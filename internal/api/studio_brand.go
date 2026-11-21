package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"videoflow-go/internal/composition"
)

var brandMu sync.Mutex

type studioBrand struct {
	Name     string `json:"name"`
	Primary  string `json:"primary"`
	Text     string `json:"text"`
	Font     string `json:"font"`
	Logo     string `json:"logo_asset_id"`
	Revision string `json:"revision"`
}

func (s *Server) handleStudioBrand(w http.ResponseWriter, r *http.Request) {
	project, e := s.activeProjectID(r.Context())
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	key := "studio_brand." + project
	brandMu.Lock()
	defer brandMu.Unlock()
	current := studioBrand{Primary: "#111827", Text: "#FFFFFF", Font: "sans-serif"}
	raw, _ := s.database.GetAppSetting(key)
	if raw != "" {
		if json.Unmarshal([]byte(raw), &current) != nil {
			http.Error(w, "saved brand is invalid", 500)
			return
		}
	}
	current.Revision = compositionHash(voiceJSON(struct{ Name, Primary, Text, Font, Logo string }{current.Name, current.Primary, current.Text, current.Font, current.Logo}))
	if r.Method == "GET" {
		writeJSON(w, 200, current)
		return
	}
	var next studioBrand
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&next) != nil || next.Revision != current.Revision {
		http.Error(w, "brand changed; reload before saving", 409)
		return
	}
	if len(next.Name) > 120 || !composition.HexColor.MatchString(next.Primary) || !composition.HexColor.MatchString(next.Text) || (next.Font != "sans-serif" && next.Font != "serif" && next.Font != "monospace") {
		http.Error(w, "choose valid brand colors and a supported font", 400)
		return
	}
	if next.Logo != "" {
		asset, e := s.database.GetAsset(next.Logo)
		if e != nil || asset.ProjectID == nil || *asset.ProjectID != project {
			http.Error(w, "logo is not in this project", 400)
			return
		}
		if _, e = s.imageURL(asset); e != nil {
			http.Error(w, "logo must be an image", 400)
			return
		}
	}
	next.Revision = ""
	if e = s.database.SetAppSetting(key, string(voiceJSON(next))); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	next.Revision = compositionHash(voiceJSON(struct{ Name, Primary, Text, Font, Logo string }{next.Name, next.Primary, next.Text, next.Font, next.Logo}))
	writeJSON(w, 200, next)
}
