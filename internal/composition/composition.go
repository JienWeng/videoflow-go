// Package composition implements VideoFlow's versioned editing interchange. It
// deliberately supports a disclosed subset rather than discarding HTML effects.
package composition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const RuntimeVersion = "0.9.33"
const SourceCommit = "e596be16c901639ce6c18032e56924a9d8e1967d"

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Manifest struct {
	CaptionBindings []CaptionBinding `json:"caption_bindings,omitempty"`
	CaptionStyle    string           `json:"caption_style"`
	Renderer        string           `json:"renderer"`
	Version         int              `json:"version"`
	Width           int              `json:"width"`
	Height          int              `json:"height"`
	FPS             int              `json:"fps"`
	Duration        float64          `json:"duration"`
	Clips           []Clip           `json:"clips"`
	Audio           []Clip           `json:"audio"`
	Captions        []Cue            `json:"captions"`
	Revision        string           `json:"revision,omitempty"`
}
type Clip struct {
	HasAudio         bool    `json:"has_audio"`
	ID               string  `json:"id"`
	AssetID          string  `json:"asset_id"`
	SHA256           string  `json:"sha256,omitempty"`
	Path             string  `json:"path,omitempty"`
	Start            float64 `json:"start"`
	Trim             float64 `json:"trim"`
	Duration         float64 `json:"duration"`
	OriginalDuration float64 `json:"original_duration"`
	Speed            float64 `json:"speed"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
}
type CaptionBinding struct {
	ClipID         string          `json:"clip_id"`
	OutputID       string          `json:"output_id"`
	CaptionSHA256  string          `json:"caption_sha256"`
	VideoSHA256    string          `json:"video_sha256"`
	SourceBindings json.RawMessage `json:"source_bindings"`
	Start          float64         `json:"start"`
	Trim           float64         `json:"trim"`
	Duration       float64         `json:"duration"`
}
type Cue struct {
	SourceClipID string  `json:"source_clip_id,omitempty"`
	CharacterID  string  `json:"character_id,omitempty"`
	Language     string  `json:"language,omitempty"`
	ID           string  `json:"id"`
	UtteranceID  string  `json:"utterance_id"`
	Start        float64 `json:"start"`
	End          float64 `json:"end"`
	Text         string  `json:"text"`
}

func ValidID(s string) bool { return safeID.MatchString(s) }
func Load(root, project, revision string) (Manifest, error) {
	var m Manifest
	if !ValidID(project) || !ValidID(revision) {
		return m, fmt.Errorf("invalid project or revision")
	}
	b, e := os.ReadFile(filepath.Join(root, project, revision, "manifest.json"))
	if e == nil {
		e = json.Unmarshal(b, &m)
	}
	return m, e
}
func Save(root, project string, m Manifest, sources map[string]string) (string, error) {
	if !ValidID(project) || m.Version != 1 || m.Width < 16 || m.Width > 3840 || m.Height < 16 || m.Height > 3840 || m.FPS < 1 || m.FPS > 60 || !(m.Duration > 0 && m.Duration <= 600) {
		return "", fmt.Errorf("invalid manifest dimensions, version or duration")
	}
	if m.CaptionStyle == "" {
		m.CaptionStyle = "default"
	}
	switch m.CaptionStyle {
	case "default", "compact", "large":
	default:
		return "", fmt.Errorf("unsupported caption style; use default, compact or large")
	}
	seen := map[string]bool{}
	font, _ := reviewedSkills.ReadFile("reviewed/hyperframes/assets/frame-presets/code-editorial/fonts/Inter-400.woff2")
	files := map[string][]byte{"assets/captions.woff2": font}
	var totalBytes int64
	prepare := func(clips []Clip) error {
		for i := range clips {
			c := &clips[i]
			if !ValidID(c.ID) || seen[c.ID] || !ValidID(c.AssetID) || c.Speed != 1 || !(c.Start >= 0 && c.Trim >= 0 && c.Duration > 0 && c.Start+c.Duration <= m.Duration) {
				return fmt.Errorf("invalid clip or unsupported speed (only 1 supported)")
			}
			seen[c.ID] = true
			if c.OriginalDuration == 0 {
				c.OriginalDuration = c.Duration
			}
			if c.Trim+c.Duration > c.OriginalDuration {
				return fmt.Errorf("trim exceeds original duration")
			}
			p, ok := sources[c.AssetID]
			if !ok {
				return fmt.Errorf("asset %s unavailable in project", c.AssetID)
			}
			ext := strings.ToLower(filepath.Ext(p))
			switch ext {
			case ".mp4", ".webm", ".mov", ".png", ".jpg", ".jpeg", ".webp", ".wav", ".mp3", ".m4a", ".ogg":
			default:
				return fmt.Errorf("unsupported asset format")
			}
			info, e := os.Lstat(p)
			if e != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("source must be a regular local file")
			}
			totalBytes += info.Size()
			if totalBytes > 256<<20 {
				return fmt.Errorf("composition source media exceeds 256 MiB")
			}
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			sum := sha256.Sum256(b)
			c.SHA256 = hex.EncodeToString(sum[:])
			c.Path = "assets/" + c.SHA256 + ext
			files[c.Path] = b
		}
		return nil
	}
	if e := prepare(m.Clips); e != nil {
		return "", e
	}
	if e := prepare(m.Audio); e != nil {
		return "", e
	}
	for _, c := range m.Captions {
		if !ValidID(c.ID) || seen[c.ID] || !(c.Start >= 0 && c.End > c.Start && c.End <= m.Duration) || strings.TrimSpace(c.Text) == "" {
			return "", fmt.Errorf("captions require unique IDs and explicit valid timings")
		}
		seen[c.ID] = true
	}
	m.Renderer = "videoflow-hyperframes-v1:" + SourceCommit
	m.Revision = ""
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	revision := hex.EncodeToString(sum[:])
	m.Revision = revision
	b, _ = json.MarshalIndent(m, "", "  ")
	dir := filepath.Join(root, project, revision)
	if e := os.MkdirAll(filepath.Join(dir, "assets"), 0700); e != nil {
		return "", e
	}
	for p, data := range files {
		if e := os.WriteFile(filepath.Join(dir, p), data, 0600); e != nil {
			return "", e
		}
	}
	if e := os.WriteFile(filepath.Join(dir, "index.html"), []byte(Document(m)), 0600); e != nil {
		return "", e
	}
	return revision, os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0600)
}
func Document(m Manifest) string {
	var b strings.Builder
	captionSize := 28
	switch m.CaptionStyle {
	case "compact":
		captionSize = 20
	case "large":
		captionSize = 40
	}
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; media-src 'self' blob:; img-src 'self' blob: data:; script-src 'self' 'unsafe-inline' blob:; connect-src 'self'; worker-src blob:; font-src 'self'; style-src 'unsafe-inline'"><style>@font-face{font-family:VideoFlowCaptions;src:url(assets/captions.woff2)}html,body{margin:0;background:#111}#root{position:relative;overflow:hidden;width:%dpx;height:%dpx}.clip{position:absolute;inset:0;width:100%%;height:100%%;object-fit:contain}.caption{inset:auto 5%% 8%%;height:auto;width:90%%;color:white;text-align:center;font:%dpx VideoFlowCaptions,sans-serif;text-shadow:0 2px 4px black}</style></head><body><div id="root" data-composition-id="root" data-no-timeline data-width="%d" data-height="%d" data-duration="%g" data-fps="%d">`, m.Width, m.Height, captionSize, m.Width, m.Height, m.Duration, m.FPS)
	for _, group := range []struct {
		clips []Clip
		audio bool
	}{{m.Clips, false}, {m.Audio, true}} {
		for _, c := range group.clips {
			tag := "video"
			if group.audio {
				tag = "audio"
			} else if strings.Contains(".png.jpg.jpeg.webp", filepath.Ext(c.Path)) {
				tag = "img"
			}
			audioAttribute := "muted"
			if c.HasAudio {
				audioAttribute = `data-has-audio="true"`
			}
			fmt.Fprintf(&b, `<%s id="%s" class="clip" src="%s" data-start="%g" data-duration="%g" data-media-start="%g" %s></%s>`, tag, c.ID, c.Path, c.Start, c.Duration, c.Trim, audioAttribute, tag)
		}
	}
	for _, c := range m.Captions {
		fmt.Fprintf(&b, `<div id="%s" class="clip caption" data-start="%g" data-duration="%g">%s</div>`, c.ID, c.Start, c.End-c.Start, html.EscapeString(c.Text))
	}
	b.WriteString("</div></body></html>")
	return b.String()
}

// Verify refuses local source edits or replaced imported media. Edits must be
// saved through a new manifest revision before any preview/render operation.
func Verify(dir string, m Manifest) error {
	font, _ := reviewedSkills.ReadFile("reviewed/hyperframes/assets/frame-presets/code-editorial/fonts/Inter-400.woff2")
	fontData, fontErr := os.ReadFile(filepath.Join(dir, "assets", "captions.woff2"))
	if fontErr != nil || string(fontData) != string(font) {
		return fmt.Errorf("caption font changed")
	}
	revision := m.Revision
	m.Revision = ""
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != revision {
		return fmt.Errorf("manifest revision mismatch")
	}
	m.Revision = revision
	b, e := os.ReadFile(filepath.Join(dir, "index.html"))
	if e != nil || string(b) != Document(m) {
		return fmt.Errorf("composition source changed; save a new manifest revision")
	}
	for _, c := range append(m.Clips, m.Audio...) {
		ext := filepath.Ext(c.Path)
		if c.Path != "assets/"+c.SHA256+ext || !safeID.MatchString(c.SHA256) {
			return fmt.Errorf("invalid asset path")
		}
		p := filepath.Join(dir, c.Path)
		info, e := os.Lstat(p)
		if e != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid source asset")
		}
		data, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != c.SHA256 {
			return fmt.Errorf("source asset changed")
		}
	}
	return nil
}
