package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"videoflow-go/internal/models"
)

func TestCharacterReferenceRegenerationFromRejectedDrafts(t *testing.T) {
	for _, mode := range []string{"approved input retained", "no approved input", "provider failure retains bindings"} {
		t.Run(mode, func(t *testing.T) {
			fail := mode == "provider failure retains bindings"
			hasApproved := mode != "no approved input"
			var images atomic.Int32
			var expectedURL string
			s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/images/models":
					w.Write([]byte(`{"data":[{"id":"openai/gpt-image-2","supported_parameters":{"aspect_ratio":{"type":"enum","values":["1:1"]},"input_references":{"type":"array"}}}]}`))
				case "/images":
					images.Add(1)
					var payload map[string]any
					json.NewDecoder(r.Body).Decode(&payload)
					raw, _ := json.Marshal(payload["input_references"])
					expectedCount := 2
					if hasApproved {
						expectedCount = 4
					}
					if strings.Count(string(raw), "image_url") != expectedCount || (hasApproved && !strings.Contains(string(raw), expectedURL)) {
						t.Error("regeneration did not use usable existing references", string(raw))
					}
					if fail {
						http.Error(w, "fixture provider failed", 500)
						return
					}
					w.Write([]byte(`{"data":[{"b64_json":"iVBORw0KGgo=","media_type":"image/png"}]}`))
				default:
					t.Errorf("unexpected provider path %s", r.URL.Path)
					w.WriteHeader(500)
				}
			})
			project, _ := s.database.GetActiveProjectID()
			session, _ := s.database.CreateDirectorSession(project, "codex", "Reference regeneration")
			unrelated := &models.Character{ProjectID: &project, Name: "Unrelated actor", Appearance: "blue skin"}
			s.database.CreateCharacter(unrelated)
			selected, _ := json.Marshal(map[string]any{"character_ids": []string{unrelated.ID}, "asset_ids": []string{}})
			s.database.SetDirectorSelection(session.ID, project, selected)
			fake := filepath.Join(t.TempDir(), "codex")
			os.WriteFile(fake, []byte(`#!/usr/bin/env python3
import json,sys
p=sys.stdin.read()
if 'Unrelated actor' in p: sys.exit(2)
print(json.dumps({'type':'thread.started','thread_id':'reference-regeneration'}))
print(json.dumps({'type':'item.completed','item':{'type':'agent_message','text':json.dumps({'prompt':'Pip with brown fur, front side and full body views'})}}))
print(json.dumps({'type':'turn.completed'}))
`), 0700)
			s.directorRunner.Codex = fake
			c := &models.Character{ProjectID: &project, Name: "Pip", Appearance: "brown fur"}
			s.database.CreateCharacter(c)
			originals := []string{}
			statuses := []string{"approved", "pending", "rejected"}
			if !hasApproved {
				statuses = []string{"pending", "rejected"}
			}
			for i, status := range statuses {
				path := filepath.Join(s.cfg.StorageRoot, "ref-"+status+".png")
				os.MkdirAll(s.cfg.StorageRoot, 0700)
				os.WriteFile(path, []byte{137, 80, 78, 71, 13, 10, 26, 10, byte(i)}, 0600)
				meta, _ := json.Marshal(map[string]string{"consistency_review": status})
				a := &models.Asset{ProjectID: &project, CharacterID: &c.ID, Type: "character_reference", Name: status, FilePath: "storage/ref-" + status + ".png", MetadataJSON: meta}
				s.database.CreateAsset(a)
				originals = append(originals, a.ID)
				if status == "approved" {
					expectedURL, _ = s.imageURL(a)
				}
			}
			c.ReferenceAssetIDsJSON, _ = json.Marshal(originals)
			s.database.UpdateCharacter(c)
			lock := make(chan struct{}, 1)
			lock <- struct{}{}
			s.referenceLocks.Store(c.ID, lock)
			duplicate := call(t, s, "POST", "/characters/"+c.ID+"/reference-sheets", `{}`)
			if duplicate.Code != 502 || images.Load() != 0 || !strings.Contains(duplicate.Body.String(), "already running") {
				t.Fatal("duplicate generation not rejected", duplicate.Code, duplicate.Body.String())
			}
			<-lock
			response := call(t, s, "POST", "/characters/"+c.ID+"/reference-sheets", `{}`)
			current, _ := s.database.GetCharacter(c.ID)
			var refs []string
			json.Unmarshal(current.ReferenceAssetIDsJSON, &refs)
			if fail {
				if response.Code != 502 || images.Load() != 1 || string(current.ReferenceAssetIDsJSON) != string(c.ReferenceAssetIDsJSON) {
					t.Fatal("failed regeneration changed bindings or did not call provider", response.Code, response.Body.String(), refs, images.Load())
				}
				return
			}
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			expectedRefs := 1
			if hasApproved {
				expectedRefs = 2
			}
			if images.Load() != 1 || len(refs) != expectedRefs || (hasApproved && refs[0] != originals[0]) {
				t.Fatal("invalid candidate bindings", refs, images.Load())
			}
			candidate, _ := s.database.GetAsset(refs[len(refs)-1])
			if candidate.CharacterID == nil || *candidate.CharacterID != c.ID || !models.ReferenceApproved(candidate.MetadataJSON) {
				t.Fatal("candidate ownership or automatic usability lost")
			}
			for _, id := range originals {
				if _, err := s.database.GetAsset(id); err != nil {
					t.Fatal("old version deleted", id, err)
				}
			}
			selection, _ := s.database.GetDirectorSelection(session.ID)
			if !strings.Contains(string(selection), unrelated.ID) {
				t.Fatal("regeneration changed the saved session selection")
			}
			graph, err := s.database.GetGraphData(project)
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range graph.Nodes {
				if node.ID == c.ID {
					var ids []string
					raw, _ := json.Marshal(node.Data["reference_asset_ids_json"])
					json.Unmarshal(raw, &ids)
					if len(ids) != expectedRefs || ids[len(ids)-1] != candidate.ID {
						t.Fatal("canvas has stale reference bindings", ids)
					}
				}
			}
			for _, edge := range graph.Edges {
				retired := false
				for i, id := range originals {
					if (!hasApproved || i > 0) && edge.Target == id {
						retired = true
					}
				}
				if edge.Source == c.ID && retired {
					t.Fatal("retired draft still shown as active reference edge")
				}
			}
			other, _ := s.database.CreateProject("Other", "")
			s.database.ActivateProject(other.ID)
			cross := call(t, s, "POST", "/characters/"+c.ID+"/reference-sheets", `{}`)
			if cross.Code != 404 || images.Load() != 1 {
				t.Fatal("cross-project regeneration accepted", cross.Code)
			}
		})
	}
}

func TestReferenceCandidatePurposeCannotBeSetByJSON(t *testing.T) {
	var selection directorSelection
	if err := json.Unmarshal([]byte(`{"character_ids":["character"],"referenceCandidate":"character","reference_candidate":"character"}`), &selection); err != nil {
		t.Fatal(err)
	}
	if selection.referenceCandidate != "" {
		t.Fatal("client enabled internal generation purpose")
	}
}
