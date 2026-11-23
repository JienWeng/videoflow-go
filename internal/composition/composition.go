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
	CaptionVariants []CaptionVariant `json:"caption_variants,omitempty"`
	Texts           []TextLayer      `json:"texts,omitempty"`
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
type CaptionVariant struct {
	Language string `json:"language"`
	Captions []Cue  `json:"captions"`
}
type TextLayer struct {
	ID         string  `json:"id"`
	Text       string  `json:"text"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Size       int     `json:"size"`
	Color      string  `json:"color"`
	Background string  `json:"background"`
	Font       string  `json:"font"`
	Position   string  `json:"position"`
}

var HexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (t TextLayer) Valid(duration float64) bool {
	return ValidID(t.ID) && strings.TrimSpace(t.Text) != "" && len(t.Text) <= 2000 && t.Start >= 0 && t.End > t.Start && t.End <= duration+0.002 && t.Size >= 12 && t.Size <= 160 && HexColor.MatchString(t.Color) && (t.Background == "transparent" || HexColor.MatchString(t.Background)) && (t.Font == "sans-serif" || t.Font == "serif" || t.Font == "monospace") && (t.Position == "top" || t.Position == "center" || t.Position == "bottom")
}
func (t TextLayer) CSS() string {
	top := "50%"
	if t.Position == "top" {
		top = "15%"
	}
	if t.Position == "bottom" {
		top = "80%"
	}
	return fmt.Sprintf("position:absolute;left:5%%;width:90%%;top:%s;transform:translateY(-50%%);text-align:center;white-space:pre-wrap;color:%s;background:%s;font:%dpx %s", top, t.Color, t.Background, t.Size, t.Font)
}

type Adjustments struct {
	Fit        string  `json:"fit"`
	Scale      float64 `json:"scale"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Brightness float64 `json:"brightness"`
	Contrast   float64 `json:"contrast"`
	Saturation float64 `json:"saturation"`
	Opacity    float64 `json:"opacity"`
}

func (a Adjustments) Valid() bool {
	return (a.Fit == "contain" || a.Fit == "cover") && a.Scale >= 1 && a.Scale <= 4 && a.X >= 0 && a.X <= 100 && a.Y >= 0 && a.Y <= 100 && a.Brightness >= 0 && a.Brightness <= 2 && a.Contrast >= 0 && a.Contrast <= 2 && a.Saturation >= 0 && a.Saturation <= 2 && a.Opacity >= 0 && a.Opacity <= 1
}
func (a Adjustments) CSS() string {
	return fmt.Sprintf("object-fit:%s;object-position:%g%% %g%%;transform:scale(%g);transform-origin:%g%% %g%%;filter:brightness(%g) contrast(%g) saturate(%g);opacity:%g", a.Fit, a.X, a.Y, a.Scale, a.X, a.Y, a.Brightness, a.Contrast, a.Saturation, a.Opacity)
}

type Clip struct {
	Overlay          *OverlayBox  `json:"overlay,omitempty"`
	Volume           *float64     `json:"volume,omitempty"`
	FadeIn           float64      `json:"fade_in,omitempty"`
	FadeOut          float64      `json:"fade_out,omitempty"`
	Muted            bool         `json:"muted,omitempty"`
	Adjustments      *Adjustments `json:"adjustments,omitempty"`
	HasAudio         bool         `json:"has_audio"`
	ID               string       `json:"id"`
	AssetID          string       `json:"asset_id"`
	SHA256           string       `json:"sha256,omitempty"`
	Path             string       `json:"path,omitempty"`
	Start            float64      `json:"start"`
	Trim             float64      `json:"trim"`
	Duration         float64      `json:"duration"`
	OriginalDuration float64      `json:"original_duration"`
	Speed            float64      `json:"speed"`
	Width            int          `json:"width"`
	Height           int          `json:"height"`
}
type OverlayBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (b OverlayBox) Valid() bool {
	return b.X >= 0 && b.Y >= 0 && b.Width > 0 && b.Height > 0 && b.X+b.Width <= 100 && b.Y+b.Height <= 100
}
func (b OverlayBox) CSS() string {
	return fmt.Sprintf("left:%g%%;top:%g%%;right:auto;bottom:auto;width:%g%%;height:%g%%", b.X, b.Y, b.Width, b.Height)
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
			if !ValidID(c.ID) || seen[c.ID] {
				return fmt.Errorf("clip id %q is missing, not allowed, or used twice", c.ID)
			}
			if !ValidID(c.AssetID) {
				return fmt.Errorf("clip %s has an invalid media id", c.ID)
			}
			if c.Speed != 1 {
				return fmt.Errorf("clip %s has speed %v; only normal speed (1) is supported", c.ID, c.Speed)
			}
			if !(c.Start >= 0 && c.Trim >= 0 && c.Duration > 0) {
				return fmt.Errorf("clip %s needs a start and trim of 0 or more and a length above 0 (start %.3fs, trim %.3fs, length %.3fs)", c.ID, c.Start, c.Trim, c.Duration)
			}
			// Times are kept to milliseconds, so a clip may end a hair past a rounded length.
			if c.Start+c.Duration > m.Duration+0.002 {
				return fmt.Errorf("clip %s ends at %.3fs, after the end of the video (%.3fs); lengthen the video or shorten the clip", c.ID, c.Start+c.Duration, m.Duration)
			}
			if (c.Volume != nil && !(*c.Volume >= 0 && *c.Volume <= 1)) || !(c.FadeIn >= 0 && c.FadeOut >= 0 && c.FadeIn+c.FadeOut <= c.Duration) {
				return fmt.Errorf("audio volume must be 0–1 and fades must fit within the clip")
			}
			if c.Adjustments != nil && !c.Adjustments.Valid() {
				return fmt.Errorf("invalid framing or color adjustment")
			}
			if c.Overlay != nil && !c.Overlay.Valid() {
				return fmt.Errorf("invalid overlay placement")
			}
			seen[c.ID] = true
			if c.OriginalDuration == 0 {
				c.OriginalDuration = c.Duration
			}
			p, ok := sources[c.AssetID]
			if !ok {
				return fmt.Errorf("asset %s unavailable in project", c.AssetID)
			}
			ext := strings.ToLower(filepath.Ext(p))
			if isStillImage(ext) {
				// A still can be held for any length; its "original" length is whatever the edit asks.
				c.Trim, c.OriginalDuration = 0, c.Duration
			}
			// Allow rounding: clip lengths are kept to milliseconds, probes are finer.
			if c.Trim+c.Duration > c.OriginalDuration+0.02 {
				return fmt.Errorf("clip %s uses %.3fs of its media (from %.3fs in, for %.3fs) but the media is only %.3fs long; make the clip shorter or trim less", c.ID, c.Trim+c.Duration, c.Trim, c.Duration, c.OriginalDuration)
			}
			switch ext {
			case ".mp4", ".webm", ".mov", ".png", ".jpg", ".jpeg", ".webp", ".wav", ".mp3", ".m4a", ".ogg", ".aac", ".flac", ".opus":
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
		if !ValidID(c.ID) || seen[c.ID] || !(c.Start >= 0 && c.End > c.Start && c.End <= m.Duration+0.002) || strings.TrimSpace(c.Text) == "" {
			return "", fmt.Errorf("captions require unique IDs and explicit valid timings")
		}
		seen[c.ID] = true
	}
	for _, t := range m.Texts {
		if !t.Valid(m.Duration) || seen[t.ID] {
			return "", fmt.Errorf("invalid text layer")
		}
		seen[t.ID] = true
	}
	if len(m.CaptionVariants) > 30 {
		return "", fmt.Errorf("too many caption variants")
	}
	for _, variant := range m.CaptionVariants {
		if strings.TrimSpace(variant.Language) == "" || len(variant.Language) > 80 || len(variant.Captions) > 500 {
			return "", fmt.Errorf("invalid caption variant")
		}
		ids := map[string]bool{}
		for _, cue := range variant.Captions {
			if !ValidID(cue.ID) || ids[cue.ID] || !(cue.Start >= 0 && cue.End > cue.Start && cue.End <= m.Duration+0.002) || strings.TrimSpace(cue.Text) == "" {
				return "", fmt.Errorf("invalid translated caption timings")
			}
			ids[cue.ID] = true
		}
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
		if e := replaceFile(filepath.Join(dir, p), data); e != nil {
			return "", e
		}
	}
	if e := replaceFile(filepath.Join(dir, "index.html"), []byte(Document(m))); e != nil {
		return "", e
	}
	return revision, replaceFile(filepath.Join(dir, "manifest.json"), b)
}

// replaceFile swaps in a complete file. Saving an edit again rewrites its
// revision, which a render may be reading; writing in place would show that
// render a truncated clip, and it would freeze on the clip's last frame.
func replaceFile(path string, data []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".saving-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e == nil {
		e = f.Chmod(0600)
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func Document(m Manifest) string {
	var b strings.Builder
	// Caption size follows the video's short side so text reads the same on a
	// 640p preview and a 1080p export (about 5% of the short side by default).
	short := m.Width
	if m.Height < short {
		short = m.Height
	}
	factor := 0.05
	switch m.CaptionStyle {
	case "compact":
		factor = 0.036
	case "large":
		factor = 0.07
	}
	captionSize := int(float64(short)*factor + 0.5)
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; media-src 'self' blob:; img-src 'self' blob: data:; script-src 'self' 'unsafe-inline' blob:; connect-src 'self'; worker-src blob:; font-src 'self' data:; style-src 'unsafe-inline'"><style>@font-face{font-family:VideoFlowCaptions;src:url(assets/captions.woff2)}html,body{margin:0;background:#111}#root{position:relative;overflow:hidden;width:%dpx;height:%dpx}.clip{position:absolute;inset:0;width:100%%;height:100%%;object-fit:contain}.caption{inset:auto 5%% 8%%;height:auto;width:90%%;color:white;text-align:center;font:700 %dpx/1.25 VideoFlowCaptions,sans-serif;text-shadow:-2px -2px 0 #000,2px -2px 0 #000,-2px 2px 0 #000,2px 2px 0 #000,0 3px 8px rgba(0,0,0,.8)}</style></head><body><div id="root" data-composition-id="root" data-no-timeline data-width="%d" data-height="%d" data-duration="%g" data-fps="%d">`, m.Width, m.Height, captionSize, m.Width, m.Height, m.Duration, m.FPS)
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
			if c.HasAudio && !c.Muted {
				audioAttribute = `data-has-audio="true"`
			}
			if c.Volume != nil {
				audioAttribute += fmt.Sprintf(` data-volume="%g"`, *c.Volume)
			}
			if c.FadeIn > 0 {
				audioAttribute += fmt.Sprintf(` data-fade-in="%g"`, c.FadeIn)
			}
			if c.FadeOut > 0 {
				audioAttribute += fmt.Sprintf(` data-fade-out="%g"`, c.FadeOut)
			}
			style := ""
			if c.Adjustments != nil && c.Adjustments.Valid() {
				style = ` style="` + c.Adjustments.CSS() + `"`
			}
			if c.Overlay != nil && c.Overlay.Valid() {
				style = ` style="` + c.Overlay.CSS() + `;` + strings.TrimSuffix(strings.TrimPrefix(style, ` style="`), `"`) + `"`
			}
			fmt.Fprintf(&b, `<%s id="%s" class="clip"%s src="%s" data-start="%g" data-duration="%g" data-media-start="%g" %s></%s>`, tag, c.ID, style, c.Path, c.Start, c.Duration, c.Trim, audioAttribute, tag)
		}
	}
	for _, c := range m.Captions {
		fmt.Fprintf(&b, `<div id="%s" class="clip caption" data-start="%g" data-duration="%g">%s</div>`, c.ID, c.Start, c.End-c.Start, html.EscapeString(c.Text))
	}
	for _, t := range m.Texts {
		if t.Valid(m.Duration) {
			fmt.Fprintf(&b, `<div id="%s" style="%s" data-start="%g" data-duration="%g">%s</div>`, t.ID, t.CSS(), t.Start, t.End-t.Start, html.EscapeString(t.Text))
		}
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
	// Edits saved before the caption font was allowed inline carry the earlier policy.
	current := Document(m)
	if earlier := strings.Replace(current, "font-src 'self' data:", "font-src 'self'", 1); e != nil || (string(b) != current && string(b) != earlier) {
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

func isStillImage(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}
