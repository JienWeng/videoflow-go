package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFishSchemaAndLicensedVoiceLibraryStayDistinct(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/schema" {
			if r.Header.Get("Authorization") != "" {
				t.Error("secret sent to public schema")
			}
			w.Write([]byte(`{"paths":{"/v1/tts":{"post":{"parameters":[{"name":"model","schema":{"enum":["future-model","s2.1-pro"],"default":"future-model"}}]}}}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer test" || r.URL.Query().Get("licensed") != "true" || r.URL.Query().Get("language") != "en" {
			t.Error("wrong voice catalog contract")
		}
		w.Write([]byte(`{"items":[{"_id":"saved-fish-voice","title":"Gentle","tags":["warm"],"languages":["en"]}],"has_more":true}`))
	}))
	defer api.Close()
	models, err := SpeechModels(context.Background(), "fish", "test", api.URL, api.URL+"/schema")
	if err != nil || len(models) != 2 || models[0].ID != "future-model" || !models[0].Default {
		t.Fatal(models, err)
	}
	voices, err := ListSpeechVoices(context.Background(), "fish", "test", api.URL, "", "en", "")
	if err != nil || len(voices.Voices) != 1 || voices.Voices[0].ID != "saved-fish-voice" || voices.Next != "2" {
		t.Fatal(voices, err)
	}
}
