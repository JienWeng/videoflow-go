package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

const FishURL = "https://api.fish.audio"

func FishModel(model string) bool {
	return model == "s2.1-pro" || model == "s2.1-pro-free" || model == "s2-pro" || model == "drama-3" || model == "s1"
}

type FishClient struct{ mediaHTTP }

func NewFishClientAt(key, base string) *FishClient {
	return &FishClient{newMediaHTTP(key, base, "Fish Audio", "Bearer ")}
}

// Speech is a single synchronous request. Never retry an ambiguous paid submission.
func (c *FishClient) Speech(ctx context.Context, model string, input map[string]any) ([]byte, error) {
	if !SafeSpeechID(model) || strings.TrimSpace(stringValue(input["text"])) == "" || strings.TrimSpace(stringValue(input["reference_id"])) == "" || utf8.RuneCountInString(stringValue(input["text"])) > 5000 {
		return nil, fmt.Errorf("Saved speech requires a supported model, saved voice ID and 1–5000 characters")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.base+"/v1/tts", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("model", model)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Fish Audio request failed; check provider activity before retrying")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Fish Audio returned HTTP %d; check account access and voice model ID", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "audio/") && resp.Header.Get("Content-Type") != "application/octet-stream" {
		return nil, fmt.Errorf("Fish Audio returned a non-audio response")
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil || len(audio) == 0 || len(audio) > 32<<20 {
		return nil, fmt.Errorf("Fish Audio returned incomplete or oversized audio")
	}
	return audio, nil
}
