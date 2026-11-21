package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
)

// OpenAISpeechModel listens to a clip; it is priced per minute of audio.
const (
	OpenAISpeechModel         = "whisper-1"
	OpenAISpeechDollarsPerMin = 0.006
)

// SpokenWord is one word heard in a clip, in the clip's own seconds.
type SpokenWord struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// TranscribeWords listens to WAV audio and returns each word with when it is spoken.
// hint is the expected script; it keeps names and spellings the way they were written.
func (c *OpenAIImages) TranscribeWords(ctx context.Context, wav []byte, language, hint string) ([]SpokenWord, error) {
	if len(wav) == 0 || len(wav) > 25<<20 {
		return nil, fmt.Errorf("audio must be between 1 byte and 25 MiB")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := [][2]string{{"model", OpenAISpeechModel}, {"response_format", "verbose_json"}, {"timestamp_granularities[]", "word"}}
	if language != "" {
		fields = append(fields, [2]string{"language", language})
	}
	if hint != "" {
		fields = append(fields, [2]string{"prompt", hint})
	}
	for _, f := range fields {
		if err := writer.WriteField(f[0], f[1]); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile("file", "clip.wav")
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(wav); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	data, err := c.http.requestBody(ctx, http.MethodPost, "/audio/transcriptions", writer.FormDataContentType(), &body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Words []SpokenWord `json:"words"`
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("OpenAI returned an unreadable transcript")
	}
	words := []SpokenWord{}
	for _, w := range result.Words {
		if w.Word != "" && w.Start >= 0 && w.End >= w.Start {
			words = append(words, w)
		}
	}
	return words, nil
}
