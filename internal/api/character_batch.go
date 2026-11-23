package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"videoflow-go/internal/models"
)

// characterSheetConcurrency is how many character sheets one batch makes at the same
// time. It matches the three background-operation slots and paid-request limit, so a
// batch never asks a picture service for more at once than the rest of the app does.
const characterSheetConcurrency = 3

// characterSheetImageCalls is how many paid picture calls one sheet takes: a single
// picture holds both the face close-up and the full-body view.
const characterSheetImageCalls = 1

// characterSheetOutcome is what happened to one character in a batch.
type characterSheetOutcome struct {
	CharacterID       string   `json:"character_id"`
	Name              string   `json:"name"`
	ReferenceAssetIDs []string `json:"reference_asset_ids,omitempty"`
	Error             string   `json:"error,omitempty"`
}

// characterSheetBatch is the result of making several characters' sheets in one go.
type characterSheetBatch struct {
	Made       []characterSheetOutcome `json:"made"`
	Failed     []characterSheetOutcome `json:"failed"`
	ImageCalls int                     `json:"image_calls"`
}

// characterSheetPlan is what a batch would make, before anything is spent.
type characterSheetPlan struct {
	Characters []characterSheetOutcome `json:"characters"`
	ImageCalls int                     `json:"image_calls"`
	Ready      bool                    `json:"ready"`
	Problem    string                  `json:"problem,omitempty"`
}

// characterHasSheet says whether c has a reference picture that is ready to use: not
// excluded, and its file is still there.
func (s *Server) characterHasSheet(c *models.Character) bool {
	for _, id := range models.IDList(c.ReferenceAssetIDsJSON) {
		asset, err := s.database.GetAsset(id)
		if err != nil || asset.ProjectID == nil || c.ProjectID == nil || *asset.ProjectID != *c.ProjectID || !models.ReferenceApproved(asset.MetadataJSON) {
			continue
		}
		if _, err = s.imageURL(asset); err == nil {
			return true
		}
	}
	return false
}

// sheetCharacters resolves which characters a batch draws. Named ids must all be in the
// project and are drawn even when they have a picture (like the per-character button);
// with none named, it is every character of the project that has no usable picture.
func (s *Server) sheetCharacters(project string, ids []string) ([]models.Character, error) {
	if len(ids) > 0 {
		chars, seen := []models.Character{}, map[string]bool{}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			c, err := s.database.GetCharacter(id)
			if err != nil || c.ProjectID == nil || *c.ProjectID != project {
				return nil, fmt.Errorf("a chosen character is not in this project")
			}
			chars = append(chars, *c)
		}
		return chars, nil
	}
	all, err := s.database.ListCharacters(project)
	if err != nil {
		return nil, err
	}
	chars := []models.Character{}
	for i := range all {
		if !s.characterHasSheet(&all[i]) {
			chars = append(chars, all[i])
		}
	}
	return chars, nil
}

// characterSheetProblem says why character sheets cannot be made right now (no picture
// key, or no local AI session to write the picture prompt), or "" when they can. It
// generates nothing.
func (s *Server) characterSheetProblem(ctx context.Context) string {
	if s.preferCodexImages(ctx) {
		return s.localTextProblem(ctx, "make character pictures")
	}
	if problem := s.referenceImageProblem(ctx); problem != "" {
		return problem
	}
	// Every picture service but OpenAI gets its prompt written by the local session first.
	if _, provider, _, _ := s.selectedMedia(ctx, "reference"); provider != "openai" {
		return s.localTextProblem(ctx, "make character pictures")
	}
	return ""
}

// planCharacterSheets prices a batch for chars and says whether it can run now.
func (s *Server) planCharacterSheets(ctx context.Context, chars []models.Character) characterSheetPlan {
	plan := characterSheetPlan{Characters: []characterSheetOutcome{}, ImageCalls: len(chars) * characterSheetImageCalls}
	for _, c := range chars {
		plan.Characters = append(plan.Characters, characterSheetOutcome{CharacterID: c.ID, Name: c.Name})
	}
	if len(chars) == 0 {
		plan.Problem = "Every character already has a reference picture."
	} else {
		plan.Problem = s.characterSheetProblem(ctx)
	}
	plan.Ready = plan.Problem == ""
	return plan
}

// makeCharacterSheets makes the sheets of chars side by side, at most
// characterSheetConcurrency at a time, each with the same steps the per-character button
// uses. One character failing never stops the others. progress, when set, hears how
// many have finished after each one. Turns on the local AI session take turns.
func (s *Server) makeCharacterSheets(ctx context.Context, chars []models.Character, onlyMissing bool, style string, progress func(done, total int)) characterSheetBatch {
	ctx = withDirectorTurnGate(ctx)
	outcomes := make([]characterSheetOutcome, len(chars))
	slots := make(chan struct{}, characterSheetConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i := range chars {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := chars[i]
			outcome := characterSheetOutcome{CharacterID: c.ID, Name: c.Name}
			select {
			case slots <- struct{}{}:
				made, err := s.generateCharacterSheet(guidedStoryContext(ctx, *c.ProjectID, []string{c.ID}), &c, onlyMissing, style)
				<-slots
				if err != nil {
					outcome.Error = err.Error()
				} else {
					outcome.ReferenceAssetIDs = models.IDList(made.ReferenceAssetIDsJSON)
				}
			case <-ctx.Done():
				outcome.Error = ctx.Err().Error()
			}
			outcomes[i] = outcome
			mu.Lock()
			done++
			if progress != nil {
				progress(done, len(chars))
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	batch := characterSheetBatch{Made: []characterSheetOutcome{}, Failed: []characterSheetOutcome{}, ImageCalls: len(chars) * characterSheetImageCalls}
	for _, outcome := range outcomes {
		if outcome.Error != "" {
			batch.Failed = append(batch.Failed, outcome)
		} else {
			batch.Made = append(batch.Made, outcome)
		}
	}
	return batch
}

// failedSheetsError names the characters whose sheet could not be made.
func failedSheetsError(failed []characterSheetOutcome) error {
	if len(failed) == 0 {
		return nil
	}
	names := []string{}
	for _, f := range failed {
		names = append(names, f.Name)
	}
	return fmt.Errorf("couldn't make the picture for %s: %s", strings.Join(names, ", "), failed[0].Error)
}

func splitIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// handleCharacterSheetsPlan (GET /characters/reference-sheets) lists the characters a
// batch would draw and its picture calls, and whether it can run now. Free.
func (s *Server) handleCharacterSheetsPlan(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	chars, err := s.sheetCharacters(project, splitIDs(r.URL.Query().Get("character_ids")))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, http.StatusOK, s.planCharacterSheets(r.Context(), chars))
}

// handleCharacterSheetsBatch (POST /characters/reference-sheets) makes several
// characters' sheets as one operation. Body: optional character_ids (default: every
// character without a usable picture) and expected_image_calls, the count the owner
// was shown; a different count means the list changed and nothing is spent.
func (s *Server) handleCharacterSheetsBatch(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	var body struct {
		CharacterIDs       []string `json:"character_ids"`
		ExpectedImageCalls *int     `json:"expected_image_calls"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", 400)
		return
	}
	chars, err := s.sheetCharacters(project, body.CharacterIDs)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	plan := s.planCharacterSheets(r.Context(), chars)
	if len(chars) == 0 {
		http.Error(w, plan.Problem, 409)
		return
	}
	if body.ExpectedImageCalls != nil && *body.ExpectedImageCalls != plan.ImageCalls {
		http.Error(w, "The characters to draw changed since you looked. Review the list and try again.", 409)
		return
	}
	if !plan.Ready {
		http.Error(w, plan.Problem, 400)
		return
	}
	op := &models.Op{Kind: "character_references", ProjectID: &project}
	s.operation(w, r, op, func(ctx context.Context) (any, error) {
		batch := s.makeCharacterSheets(ctx, chars, false, "", func(done, total int) {
			if op.ID == "" {
				return
			}
			raw, _ := json.Marshal(map[string]any{"stage": fmt.Sprintf("Making character pictures: %d of %d done", done, total), "done": done, "total": total})
			_ = s.database.UpdateOp(op.ID, models.StatusRunning, raw, nil)
		})
		if len(batch.Made) == 0 {
			return batch, failedSheetsError(batch.Failed)
		}
		return batch, nil
	})
}
