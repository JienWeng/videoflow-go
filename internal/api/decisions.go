package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"videoflow-go/internal/agents"
	"videoflow-go/internal/db"
)

// The decision log is the project's memory: choices already made (language, style,
// format, length, platform, music, each character's voice, proposals the user turned
// down). Every chat turn carries it under decided_so_far, and the assistant must not
// ask about an entry again unless the user changes it.
//
// Entries are written:
//   - by the app, as the user acts: confirming the creative brief (language, style_id,
//     aspect_ratio, target_duration, platform, music, conversation_mode), saving an
//     approved voice (voice:<character_id>), saving a voice sample
//     (voice_sample:<character_id>), picking a quick reply to a question (the
//     question's key, or choice:<question>), and dismissing a proposal
//     (rejected:<action>:<target>, sent by the chat panel);
//   - by the assistant, through its reply's optional remember list, limited to
//     assistantDecisionKeys and never replacing what the user decided;
//   - by the user, through PUT and DELETE /decisions/{key}.

// Limits on decision values.
const (
	maxDecisionValueChars = 500
	maxRememberPerTurn    = 3
	maxRememberValueChars = 300
	// maxChatDecisions is how many entries, newest first, a chat turn carries.
	maxChatDecisions      = 60
	maxChatDecisionValue  = 200
	decidedSoFarRule      = "Decided so far — do not ask again unless the user changes it"
	maxAnswerQuestionChar = 300
)

// assistantDecisionKeys are the keys the assistant may remember; a key with ":" is a
// prefix followed by an id, such as voice:<character_id>.
var assistantDecisionKeys = map[string]bool{
	"language": true, "style_id": true, "aspect_ratio": true, "target_duration": true, "platform": true,
	"music": true, "conversation_mode": true, "tone": true, "audience": true, "resolution": true,
	"notes": true, "reviewed": true, "voice:": true, "character:": true, "scene:": true, "notes:": true,
}

// assistantMayRemember says whether the assistant may write key.
func assistantMayRemember(key string) bool {
	if !validDecisionKey(key) {
		return false
	}
	if prefix, _, found := strings.Cut(key, ":"); found {
		return assistantDecisionKeys[prefix+":"]
	}
	return assistantDecisionKeys[key]
}

// rememberDecisions reads the first maxRememberPerTurn entries of the planner's
// remember list and returns the ones saved. Invalid entries are skipped (they still
// count toward the limit), and a decision the user made is never replaced.
func (s *Server) rememberDecisions(projectID string, items []agents.RememberItem) []db.ProjectDecision {
	saved := []db.ProjectDecision{}
	for i, item := range items {
		if i == maxRememberPerTurn {
			break
		}
		key, value := strings.TrimSpace(item.Key), clip(item.Value, maxRememberValueChars)
		if value == "" || !assistantMayRemember(key) {
			continue
		}
		if ok, err := s.database.SetDecision(projectID, key, value, db.DecisionByAssistant, true); err == nil && ok {
			saved = append(saved, db.ProjectDecision{ProjectID: projectID, Key: key, Value: value, Source: db.DecisionByAssistant})
		}
	}
	return saved
}

// recordDecision keeps what the user just decided. Recording never fails the request
// it follows: a full or unwritable log only means the assistant may ask again.
func (s *Server) recordDecision(projectID, key, value string) {
	value = clip(value, maxDecisionValueChars)
	if !validDecisionKey(key) || value == "" {
		return
	}
	if _, err := s.database.SetDecision(projectID, key, value, db.DecisionByUser, false); err != nil {
		log.Printf("decision log: could not keep %s: %v", key, err)
	}
}

// forgetDecision removes an entry the user's own change made untrue.
func (s *Server) forgetDecision(projectID, key string) {
	if _, err := s.database.DeleteDecision(projectID, key); err != nil {
		log.Printf("decision log: could not remove %s: %v", key, err)
	}
}

// recordBriefDecisions keeps the choices of a creative brief the user confirmed.
func (s *Server) recordBriefDecisions(projectID string, b generationBrief) {
	for key, value := range map[string]string{
		"language": b.Language, "style_id": b.PresetID, "aspect_ratio": b.AspectRatio, "platform": b.Platform,
		"music": b.Music, "conversation_mode": b.ConversationMode,
	} {
		if strings.TrimSpace(value) != "" {
			s.recordDecision(projectID, key, value)
		}
	}
	if b.TargetDuration > 0 {
		s.recordDecision(projectID, "target_duration", strconv.Itoa(b.TargetDuration)+" seconds")
	}
}

// chatAnswer is a quick reply the user picked: the question it answers and the option.
type chatAnswer struct {
	Key      string `json:"key"`
	Question string `json:"question"`
	Label    string `json:"label"`
	Value    string `json:"value"`
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// answerKey is the decision-log key a quick reply settles: the question's own key, or
// choice:<the question in a few words>.
func answerKey(a chatAnswer) string {
	if key := strings.TrimSpace(a.Key); validDecisionKey(key) {
		return key
	}
	slug := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(clip(a.Question, maxAnswerQuestionChar)), "_"), "_")
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "_")
	}
	if slug == "" {
		slug = "answer"
	}
	return "choice:" + slug
}

// recordAnswer keeps the quick reply the user picked.
func (s *Server) recordAnswer(projectID string, a *chatAnswer) {
	if a == nil {
		return
	}
	value := strings.TrimSpace(a.Label)
	if v := strings.TrimSpace(a.Value); v != "" && !strings.EqualFold(v, value) {
		value = strings.TrimSpace(value + " — " + v)
	}
	s.recordDecision(projectID, answerKey(*a), clip(value, maxRememberValueChars))
}

// chatDecidedSoFar is the decision log as a chat turn carries it, newest first.
func chatDecidedSoFar(decisions []db.ProjectDecision) map[string]any {
	items := []map[string]string{}
	for i, d := range decisions {
		if i == maxChatDecisions {
			break
		}
		items = append(items, map[string]string{"key": d.Key, "value": clip(d.Value, maxChatDecisionValue), "by": d.Source})
	}
	return map[string]any{"rule": decidedSoFarRule, "decisions": items}
}

// changeWords mark a message that changes an earlier choice, so a question about it
// is the user's own doing and stands: English, Spanish and French words, and Chinese
// characters (which have no word boundaries). Other languages are not recognised, so
// there a question the log settles is overruled even when the user asks for a change.
// A word matched by mistake only lets the assistant ask once more.
var changeWords = regexp.MustCompile(`(?i)\b(change|changing|different|instead|switch|another|rather|redo|other|` +
	`c[aá]mbi\w*|diferente|distint[oa]|otr[oa]s?|en vez|en lugar|rehac\w*|rehaz|` +
	`chang(e|er|eons|ez)|autres?|diff[ée]rente?s?|plut[oô]t|au lieu|refai\w*)\b|改|换|更换|另外|别的|不同|而是`)

// applySettledAsk overrules a question the decision log already answers: with a step
// to take, the assistant goes ahead with the earlier choice, unless the user's message
// is about changing it. The plan confirmation and a destructive step are always asked.
func applySettledAsk(d chatDecision, action, message string, decisions []db.ProjectDecision) chatDecision {
	if d.Mode != decisionAsk || d.Key == "" || action == "unknown" || d.Reason == askConfirmPlan || d.Reason == askDestructive || changeWords.MatchString(message) {
		return d
	}
	for _, entry := range decisions {
		if entry.Key == d.Key {
			return overruleAsk(d, decisionAct, "already_decided", clip("Kept your earlier choice: "+entry.Key+" = "+entry.Value, maxAssumptionChars))
		}
	}
	return d
}

func (s *Server) registerDecisionRoutes() {
	s.router.Get("/decisions", s.handleListDecisions)
	s.router.Put("/decisions/{key}", s.handlePutDecision)
	s.router.Delete("/decisions/{key}", s.handleDeleteDecision)
}

func (s *Server) handleListDecisions(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	list, err := s.database.ListDecisions(project)
	if err != nil {
		http.Error(w, "could not read the decision log", 500)
		return
	}
	writeJSON(w, 200, map[string]any{"project_id": project, "decisions": list})
}

// decisionKeyParam is the {key} of the route. chi gives it as sent, so an escaped
// ":" (%3A) is decoded here.
func decisionKeyParam(r *http.Request) (string, bool) {
	key, err := url.PathUnescape(chi.URLParam(r, "key"))
	key = strings.TrimSpace(key)
	return key, err == nil && validDecisionKey(key)
}

func (s *Server) handlePutDecision(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	key, ok := decisionKeyParam(r)
	if !ok {
		http.Error(w, "a decision key is a lowercase name, optionally followed by :-separated ids", 400)
		return
	}
	var body struct {
		Value string `json:"value"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil || strings.TrimSpace(body.Value) == "" {
		http.Error(w, "send the decision as {\"value\": \"...\"}", 400)
		return
	}
	if len([]rune(strings.TrimSpace(body.Value))) > maxDecisionValueChars {
		http.Error(w, "a decision is at most 500 characters", 400)
		return
	}
	if _, err = s.database.SetDecision(project, key, strings.TrimSpace(body.Value), db.DecisionByUser, false); err != nil {
		if errors.Is(err, db.ErrDecisionLogFull) {
			http.Error(w, err.Error(), 409)
			return
		}
		http.Error(w, "could not save the decision", 500)
		return
	}
	saved, err := s.database.GetDecision(project, key)
	if err != nil {
		http.Error(w, "could not save the decision", 500)
		return
	}
	writeJSON(w, 200, saved)
}

func (s *Server) handleDeleteDecision(w http.ResponseWriter, r *http.Request) {
	project, err := s.activeProjectID(r.Context())
	if err != nil {
		http.Error(w, "no active project", 400)
		return
	}
	key, ok := decisionKeyParam(r)
	if !ok {
		http.Error(w, "unknown decision key", 400)
		return
	}
	if _, err = s.database.DeleteDecision(project, key); err != nil {
		http.Error(w, "could not delete the decision", 500)
		return
	}
	w.WriteHeader(204)
}
