package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"

	"videoflow-go/internal/models"
)

// Full auto's keyframes, continuity and approvals.
//
// Keyframes: a scene made from a first frame needs its opening picture (the storyboard
// keyframe). Full auto makes every missing one in story order BEFORE any clip, so the
// picture check sees it and nothing is paid for twice: a scene whose keyframe is current
// is skipped, and the render then reuses it. A scene made from reference pictures needs
// no keyframe.
//
// Continuity: each keyframe is drawn with the previous scene's picture as one more
// reference (the last frame of its finished clip when there is one, else its keyframe)
// and told to continue it: same characters, look, light and positions. So clips made
// from those keyframes start where the story left off.
//
// Approvals: once the clips are checked, every clip that passed is approved; the export
// needs every clip in the edit approved (clip_approval.go).

// autopilotKeyframeMaker draws and attaches one keyframe; tests replace it.
var autopilotKeyframeMaker = (*Server).makeContinuityKeyframe

// continuityNote is what a keyframe is told about the scene before it.
func continuityNote(previous string, fromClip bool) string {
	if previous == "" {
		return ""
	}
	source := "its opening keyframe"
	if fromClip {
		source = "the last frame of its clip"
	}
	return fmt.Sprintf("\nContinuity: this scene follows straight on from “%s”; the last reference image is %s. Keep the same characters, faces, clothes, look, lighting and where everyone stands, unless this scene says otherwise.", previous, source)
}

// makeContinuityKeyframe draws a scene's keyframe with the previous scene's picture
// (a data URL, or "") as a continuity reference, and attaches it to the scene.
func (s *Server) makeContinuityKeyframe(ctx context.Context, scene *models.Scene, previousFrame, note string) (string, error) {
	unlock, err := s.lockSceneRender(scene.ID)
	if err != nil {
		return "", err
	}
	defer unlock()
	current, err := s.database.GetScene(scene.ID)
	if err != nil {
		return "", err
	}
	if previousFrame != "" {
		ctx = context.WithValue(ctx, continuityFrameKey{}, previousFrame)
	}
	assets, err := s.storyboardCandidates(guidedStoryContext(ctx, *current.ProjectID, nil), current, note)
	if err != nil {
		return "", err
	}
	if _, err = s.attachStoryboard(current, assets); err != nil {
		return "", err
	}
	*scene = *current
	return assets[0].ID, nil
}

// continuityFrameKey carries the previous scene's picture into its next keyframe.
type continuityFrameKey struct{}

// withContinuityFrame adds the previous scene's picture to a keyframe's references.
func withContinuityFrame(ctx context.Context, refs []string) []string {
	if frame, _ := ctx.Value(continuityFrameKey{}).(string); frame != "" {
		if len(refs) >= 8 {
			refs = refs[:7] // image services take at most 8 references
		}
		return append(refs, frame)
	}
	return refs
}

// previousScenePicture is the picture the next scene continues from: the last frame of
// the scene's current clip when it has one, else its keyframe. "" when it has neither.
func (s *Server) previousScenePicture(ctx context.Context, scene *models.Scene) (string, bool) {
	jobs, _ := s.database.ListRenderJobs(*scene.ProjectID) // no jobs: fall back to the keyframe
	if current := currentSceneClipJobs(jobs)[scene.ID]; len(current) > 0 {
		last := current[len(current)-1]
		if outs, _ := s.database.ListOutputsForJob(last.ID); len(outs) > 0 {
			if frame := s.lastClipFrame(ctx, outs[len(outs)-1].VideoPath); frame != "" {
				return frame, true
			}
		}
	}
	if fresh, err := s.database.GetScene(scene.ID); err == nil {
		ids := models.IDList(fresh.AssetIDsJSON)
		for i := len(ids) - 1; i >= 0; i-- {
			if asset, err := s.database.GetAsset(ids[i]); err == nil && asset.Type == "storyboard" && models.ReferenceApproved(asset.MetadataJSON) {
				if url, err := s.imageURL(asset); err == nil {
					return url, false
				}
			}
		}
	}
	return "", false
}

// lastClipFrame is a clip's last frame as a data URL, or "" when it can't be read.
func (s *Server) lastClipFrame(ctx context.Context, videoPath string) string {
	full, err := s.storagePath(videoPath)
	if err != nil {
		return ""
	}
	dir, err := os.MkdirTemp("", "videoflow-continuity-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)
	frames, err := selfCheckFrames(ctx, full, dir)
	if err != nil || len(frames) == 0 {
		return ""
	}
	data, err := os.ReadFile(frames[len(frames)-1])
	if err != nil || len(data) == 0 {
		return ""
	}
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// makeKeyframes is Full auto's keyframe step, before the picture check and any clip.
func (a *autopilotRunner) makeKeyframes(ctx context.Context, adopted map[string][]string) error {
	if !a.body.FullAuto || a.body.JoinOnly {
		return nil
	}
	needs, reused, references := []int{}, 0, 0
	for i := range a.scenes {
		scene := &a.scenes[i]
		if len(adopted[scene.ID]) > 0 {
			continue
		}
		plan, err := a.s.sceneProductionPlan(ctx, scene)
		if err != nil || plan.Blocked != "" || plan.Calls == 0 {
			continue // a plan problem is reported when the clip is started; made clips are kept
		}
		switch {
		case plan.VideoInput == inputReferences:
			references++
		case plan.StoryboardReady:
			reused++
		default:
			needs = append(needs, i)
		}
	}
	a.notes.keyframesReused, a.notes.referenceScenes = reused, references
	if len(needs) == 0 {
		if reused+references > 0 {
			a.stage(keyframeSkipText(reused, references))
		}
		return nil
	}
	for n, i := range needs {
		if err := a.sameProject(); err != nil {
			return err
		}
		scene := &a.scenes[i]
		previous, frame, fromClip := "", "", false
		if i > 0 {
			previous = a.scenes[i-1].Title
			frame, fromClip = a.s.previousScenePicture(ctx, &a.scenes[i-1])
			if frame == "" {
				previous = ""
			}
		}
		text := fmt.Sprintf("Making keyframe %d of %d: the opening picture of “%s”, which its video starts from", n+1, len(needs), scene.Title)
		if previous != "" {
			text += fmt.Sprintf(", continuing from “%s”", previous)
		}
		a.stage(text)
		if _, err := autopilotKeyframeMaker(a.s, ctx, scene, frame, continuityNote(previous, fromClip)); err != nil {
			return fmt.Errorf("couldn't make the keyframe for “%s”: %w. The keyframes already made are kept; ask me again to carry on, and no video was paid for", scene.Title, err)
		}
		a.notes.keyframesMade++
	}
	return nil
}

// keyframeSkipText says why no keyframe was drawn.
func keyframeSkipText(reused, references int) string {
	parts := []string{}
	if reused > 0 {
		parts = append(parts, fmt.Sprintf("%d scene%s already %s a current keyframe (reused, not paid again)", reused, pluralS(reused), map[bool]string{true: "has", false: "have"}[reused == 1]))
	}
	if references > 0 {
		parts = append(parts, fmt.Sprintf("%d scene%s use%s reference pictures, so %s no keyframe", references, pluralS(references), map[bool]string{true: "s", false: ""}[references == 1], map[bool]string{true: "it needs", false: "they need"}[references == 1]))
	}
	return "Keyframes: " + strings.Join(parts, "; ")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// autopilotNotes collect what the run did, for its closing summary.
type autopilotNotes struct {
	keyframesMade, keyframesReused, referenceScenes int
	approval                                        *clipApprovalReport
}

// approveRunClips approves every clip of the run's scenes that passed its checks.
func (a *autopilotRunner) approveRunClips() (clipApprovalReport, error) {
	ids := []string{}
	for _, scene := range a.scenes {
		ids = append(ids, scene.ID)
	}
	by := approvalByFinish
	if a.body.FullAuto {
		by = approvalByFullAuto
		a.stage("Approving the clips that passed every check")
	} else {
		a.stage("Approving the finished clips")
	}
	report, err := a.s.approveClips(a.project, approveClipsRequest{SceneIDs: ids}, by)
	a.notes.approval = &report
	return report, err
}

// summary is the run's closing report, in words.
func (a *autopilotRunner) summary(final bool) string {
	parts := []string{}
	if n := a.notes.keyframesMade; n > 0 {
		parts = append(parts, fmt.Sprintf("made %d keyframe%s, each continuing the scene before", n, pluralS(n)))
	}
	if n := a.notes.keyframesReused; n > 0 {
		parts = append(parts, fmt.Sprintf("reused %d keyframe%s", n, pluralS(n)))
	}
	parts = append(parts, fmt.Sprintf("made the clips of %d scene%s in story order", len(a.scenes), pluralS(len(a.scenes))))
	if a.body.FullAuto {
		parts = append(parts, "checked every picture and clip")
	}
	if r := a.notes.approval; r != nil {
		parts = append(parts, fmt.Sprintf("approved %d clip%s", r.approvedCount(), pluralS(r.approvedCount())))
		if n := len(r.Skipped); n > 0 {
			parts = append(parts, fmt.Sprintf("left %d for you to review", n))
		}
	}
	if final {
		parts = append(parts, "joined the scenes in story order with captions and exported the final video")
	}
	text := strings.Join(parts, ", ")
	return strings.ToUpper(text[:1]) + text[1:] + "."
}
