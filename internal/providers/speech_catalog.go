package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ElevenURL = "https://api.elevenlabs.io"

type SpeechModel struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Default   bool     `json:"default"`
	Languages []string `json:"languages,omitempty"`
}
type SpeechVoice struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Labels      map[string]string `json:"labels"`
	Preview     string            `json:"preview_url,omitempty"`
}
type SpeechVoices struct {
	Voices []SpeechVoice `json:"voices"`
	Next   string        `json:"next_page,omitempty"`
}

func speechJSON(ctx context.Context, key, base, provider, path string, out any) error {
	c := newMediaHTTP(key, base, provider, "")
	c.client.Timeout = 20 * time.Second
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	if key != "" {
		if provider == "elevenlabs" {
			req.Header.Set("xi-api-key", key)
		} else {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s catalog request failed", provider)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("%s catalog returned HTTP %d", provider, res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(b) > 4<<20 {
		return fmt.Errorf("catalog response is too large or incomplete")
	}
	return json.Unmarshal(b, out)
}
func SpeechModels(ctx context.Context, provider, key, base, schemaURL string) ([]SpeechModel, error) {
	models := []SpeechModel{}
	if provider == "elevenlabs" {
		var list []struct {
			ID        string `json:"model_id"`
			Name      string `json:"name"`
			TTS       bool   `json:"can_do_text_to_speech"`
			Alpha     bool   `json:"requires_alpha_access"`
			Languages []struct {
				ID string `json:"language_id"`
			} `json:"languages"`
		}
		if err := speechJSON(ctx, key, base, provider, "/v1/models", &list); err != nil {
			return nil, err
		}
		for _, m := range list {
			if m.TTS && !m.Alpha && SafeSpeechID(m.ID) {
				langs := []string{}
				for _, l := range m.Languages {
					langs = append(langs, l.ID)
				}
				models = append(models, SpeechModel{ID: m.ID, Name: m.Name, Default: m.ID == "eleven_multilingual_v2", Languages: langs})
			}
		}
		return models, nil
	}
	if provider != "fish" {
		return nil, fmt.Errorf("choose Fish Audio or ElevenLabs for live speech catalogs")
	}
	var schema struct {
		Paths map[string]struct {
			Post struct {
				Parameters []struct {
					Name   string `json:"name"`
					Schema struct {
						Default string   `json:"default"`
						Enum    []string `json:"enum"`
					} `json:"schema"`
				} `json:"parameters"`
			} `json:"post"`
		} `json:"paths"`
	}
	if err := speechJSON(ctx, "", schemaURL, "fish", "", &schema); err != nil {
		return nil, err
	}
	for _, p := range schema.Paths["/v1/tts"].Post.Parameters {
		if p.Name == "model" {
			for _, id := range p.Schema.Enum {
				if SafeSpeechID(id) {
					models = append(models, SpeechModel{ID: id, Name: id, Default: id == p.Schema.Default})
				}
			}
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("official Fish schema contains no speech model IDs")
	}
	return models, nil
}
func SafeSpeechID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}
func ListSpeechVoices(ctx context.Context, provider, key, base, search, language, page string) (SpeechVoices, error) {
	result := SpeechVoices{Voices: []SpeechVoice{}}
	q := url.Values{}
	if provider == "elevenlabs" {
		q.Set("page_size", "100")
		if search != "" {
			q.Set("search", search)
		}
		if language != "" {
			q.Set("language", language)
		}
		if page != "" {
			q.Set("next_page_token", page)
		}
		var raw struct {
			Voices []struct {
				ID          string            `json:"voice_id"`
				Name        string            `json:"name"`
				Description string            `json:"description"`
				Labels      map[string]string `json:"labels"`
				Preview     string            `json:"preview_url"`
			} `json:"voices"`
			Next string `json:"next_page_token"`
			More bool   `json:"has_more"`
		}
		if err := speechJSON(ctx, key, base, provider, "/v2/voices?"+q.Encode(), &raw); err != nil {
			return result, err
		}
		for _, v := range raw.Voices {
			result.Voices = append(result.Voices, SpeechVoice{ID: v.ID, Name: v.Name, Description: v.Description, Labels: v.Labels, Preview: v.Preview})
		}
		if raw.More {
			result.Next = raw.Next
		}
		return result, nil
	}
	q.Set("page_size", "100")
	q.Set("page_number", "1")
	q.Set("licensed", "true")
	if page != "" {
		q.Set("page_number", page)
	}
	if search != "" {
		q.Set("title", search)
	}
	if language != "" {
		q.Set("language", language)
	}
	var raw struct {
		Items []struct {
			ID          string   `json:"_id"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
			Languages   []string `json:"languages"`
		} `json:"items"`
		More bool `json:"has_more"`
	}
	if provider != "fish" {
		return result, fmt.Errorf("unsupported voice catalog")
	}
	if err := speechJSON(ctx, key, base, provider, "/model?"+q.Encode(), &raw); err != nil {
		return result, err
	}
	for _, v := range raw.Items {
		result.Voices = append(result.Voices, SpeechVoice{ID: v.ID, Name: v.Title, Description: v.Description, Labels: map[string]string{"tags": strings.Join(v.Tags, " "), "language": strings.Join(v.Languages, " ")}})
	}
	if raw.More {
		n := 1
		fmt.Sscanf(q.Get("page_number"), "%d", &n)
		result.Next = fmt.Sprint(n + 1)
	}
	return result, nil
}
func ElevenSpeech(ctx context.Context, key, base, model string, input map[string]any) ([]byte, error) {
	if !SafeSpeechID(model) || !SafeSpeechID(stringValue(input["reference_id"])) || strings.TrimSpace(stringValue(input["text"])) == "" {
		return nil, fmt.Errorf("ElevenLabs speech needs an explicit model, voice ID and text")
	}
	c := newMediaHTTP(key, base, "ElevenLabs", "")
	payload, _ := json.Marshal(map[string]any{"text": input["text"], "model_id": model})
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/v1/text-to-speech/"+url.PathEscape(stringValue(input["reference_id"]))+"?output_format=mp3_44100_128", strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ElevenLabs request failed; check provider activity before retrying")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("ElevenLabs returned HTTP %d; check account, model and voice access", res.StatusCode)
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "audio/") {
		return nil, fmt.Errorf("ElevenLabs returned a non-audio response")
	}
	audio, err := io.ReadAll(io.LimitReader(res.Body, (32<<20)+1))
	if err != nil || len(audio) == 0 || len(audio) > 32<<20 {
		return nil, fmt.Errorf("ElevenLabs audio is incomplete or oversized")
	}
	return audio, nil
}
