---
name: studio-recovery
description: Diagnose a blocked VideoFlow production, explain unexpected results, and guide the user from the last usable output to the smallest repair.
---

# Recover a Studio workflow

Use this when someone asks what went wrong, what is ready, why a result looks wrong, or how to continue interrupted work.

Start with the selected project and scene. Match the user's symptom to a saved operation, render job, linked asset or current scene setup. A recent failure on another scene is only a lead. An empty activity list may be incomplete. When the evidence is missing, say what you need and point to the specific Activity entry or scene control. Never ask for a password, device code or API key in chat.

Tell the user what happened, what work they still have, and the next useful action. Use their scene and character names. Offer one compatible action card when available. Use a destination in the app when the repair requires a setting or manual edit. Mention potential provider use before generation. After a run, use the returned result and saved state to explain what changed. An accepted request or an operation marked succeeded can still contain a visual verdict of needs_attention.

| Where the flow stopped | Evidence to check | Smallest useful recovery |
| --- | --- | --- |
| Project, selection or chat history | Active project, selected item and session, interrupted turn, proposal revision | Restore the intended session. For a missing or foreign character, refresh the project selection and prepare a fresh proposal. Preserve the conversation and existing assets. |
| Backend connection or stalled progress | Activity status, saved output and provider job ID if available | Reconnect and inspect the existing job. A lost connection leaves the outcome unknown. Reconcile before offering a retry that might spend credits again. |
| Local planning or provider setup | Selected engine, CLI detection, account sign-in, provider route, model capabilities | Guide local sign-in through AI sessions or provider setup through Settings. CLI detection alone cannot verify login. Claude plans with a configured image API. Codex can use native images, with fallback on established unsupported input or unavailability. An ambiguous native attempt needs reconciliation. |
| Story brief or shot plan | Confirmed style, delivery, language, scene count, duration, saved shots and model limits | Ask only for missing decisions. Preserve custom entries. For incomplete shots or invalid timing, repair that scene's plan and verify shot durations against the scene before any render. |
| Cast, props or location | Scene connections, existing identities, missing or excluded references | Reuse the linked item when appropriate. Generate the missing character sheet or scene asset through the corresponding card. Confirm the resulting asset ID is bound to the intended scene or character. |
| Style drift, framing or stale storyboard | Project look, scene look, character reference medium, shot prompts, linked storyboard revision and requested aspect | Offer audit_storyboard on the intended scene. Inspection checks the pixels; optional repair makes one candidate and checks it. Report its verdict. If it still fails, address the conflicting reference or direction before generating again. Verify actual dimensions separately when framing matters. Treat historical accuracy as unverified without supporting sources. |
| Video, voices or unexpected cost | Has shots, current storyboard, native/saved/silent audio, speaker setup, render grouping, displayed calls and seconds | Use the scene render card. Resolve missing saved voices there when needed. Reuse matching clips. Let the user see the paid-call plan before starting video. A shot count alone does not establish the charge. |
| Assembly, music, captions or export | Completed source clips, final audio, cue timing, saved edit revision and export result | Continue from existing clips in the composition editor. Use actual audio for captions, verify timing and play the exported result before reporting audiovisual success. |
| Unknown or repeated failure | Last successful stage, exact failing stage, affected scene, whether an external request was submitted | Preserve completed work. Give the Activity entry and one concrete check. If the cause remains unknown, say so. An uncertain payment or repeated identical failure ends automatic attempts. |

Example reply when the evidence supports it: “The opening scene still has its two shots. Its storyboard predates your documentary look. Choose **Run visual audit** to compare the image with the scene. Enable **Repair if needed** if you want one replacement.”
