package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"videoflow-go/internal/models"
)

func TestCharacterReferenceUploadAppearsOnCharacter(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()
	router := server.Router()
	create := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/characters", bytes.NewBufferString(`{"name":"Reference fixture"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, req)
	if create.Code != 201 {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	var character models.Character
	json.Unmarshal(create.Body.Bytes(), &character)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, _ := form.CreateFormFile("file", "reference.png")
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO1cAAAAASUVORK5CYII=")
	file.Write(png)
	form.WriteField("character_id", character.ID)
	form.Close()
	upload := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/assets/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	router.ServeHTTP(upload, req)
	if upload.Code != 200 {
		t.Fatalf("upload: %d %s", upload.Code, upload.Body.String())
	}
	var asset models.Asset
	json.Unmarshal(upload.Body.Bytes(), &asset)
	if !strings.HasPrefix(asset.FilePath, "storage/assets/") {
		t.Fatalf("upload must return public media URL under arbitrary storage roots: %q", asset.FilePath)
	}
	mediaResponse := httptest.NewRecorder()
	router.ServeHTTP(mediaResponse, httptest.NewRequest("GET", "/"+asset.FilePath, nil))
	if mediaResponse.Code != 200 || !bytes.Equal(mediaResponse.Body.Bytes(), png) {
		t.Fatal("uploaded public URL does not serve the original image", mediaResponse.Code)
	}
	result := httptest.NewRecorder()
	router.ServeHTTP(result, httptest.NewRequest("GET", "/characters/"+character.ID, nil))
	json.Unmarshal(result.Body.Bytes(), &character)
	var refs []string
	json.Unmarshal(character.ReferenceAssetIDsJSON, &refs)
	if len(refs) != 1 || refs[0] != asset.ID {
		t.Fatalf("uploaded photo missing from character references: %v", refs)
	}
	if asset.CharacterID == nil || *asset.CharacterID != character.ID {
		t.Fatal("asset missing character association")
	}
}

func TestReferenceUploadRejectsUnknownCharacterWithoutSavingAsset(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, _ := form.CreateFormFile("file", "reference.png")
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aO1cAAAAASUVORK5CYII=")
	file.Write(png)
	form.WriteField("character_id", "char_missing")
	form.Close()
	req := httptest.NewRequest("POST", "/assets/upload", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	result := httptest.NewRecorder()
	server.Router().ServeHTTP(result, req)
	if result.Code != 400 {
		t.Fatalf("invalid character accepted: %d", result.Code)
	}
	result = httptest.NewRecorder()
	server.Router().ServeHTTP(result, httptest.NewRequest("GET", "/assets", nil))
	var assets []models.Asset
	json.Unmarshal(result.Body.Bytes(), &assets)
	if len(assets) != 0 {
		t.Fatal("failed upload left orphaned assets")
	}
}
