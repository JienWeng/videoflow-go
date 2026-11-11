# Canvas-first VideoFlow redesign

Status: proposed for user review. No product implementation authorized by approval of this document yet.
Repository: JienWeng/videoflow-go.

## Intent and decisions

The user selected Canvas + chat as the main workspace: click a character, scene or shot to edit it with contextual prompts. The primary audience is a creator who wants consistent characters and assets and an understandable path to a finished video without navigating implementation-oriented pages.

Success: a first-time user can select an engine, start a story, inspect and edit a selected entity, review keyframes and deliberately render a video, while understanding what is selected, saved, running and potentially chargeable.

## Approaches

1. Canvas + contextual chat (selected): preserve spatial storytelling, expose the current selection and its editable information, reduce page switching. Requires careful separation of field edits and chat proposals.
2. Guided creator (considered, not selected): simpler linear onboarding, but makes iterative graph editing secondary. Keep existing Create as an optional entry, not the default workspace.
3. Cosmetic refresh only (rejected as insufficient): typography and color alone do not resolve hidden engine setup, ambiguous selection or fragmented editing.

## Workspace and navigation

Desktop: compact project navigation, central graph, one right-hand contextual workbench. Project toolbar shows active project, engine/session, connection state and activity. Primary destinations are Studio, Library and Videos. Library groups Characters and Assets; secondary tools retain their existing URLs via a clearly labelled Tools menu. Voices & captions, Compositions, Create, Sessions and Settings stay reachable. No feature deletion or renamed backend API.

Right workbench has Details and Chat tabs and a persistent selected-item header. Details shows the current values; Chat shows a compact reference to that same selection, with an expandable summary of prompt, cast, asset locks and voice context. Selecting a node changes context, never submits a message or overwrites a typed draft. Closing selection returns chat to project scope. A clear scope label distinguishes project versus selected item. Old action proposals retain their original target and existing stale-context checks.

Mobile: project header and explicit navigation drawer; Studio switches between Canvas and Workbench rather than shrinking both into two simultaneous panels. Details/Chat remain available inside Workbench. A selected-item indicator and Back to canvas control preserve orientation. All destinations stay accessible. Drawer dismisses on navigation/Escape and restores focus.

## Click-to-edit behavior

Scene: edit title, summary, duration, aspect and existing script fields where supported. Shot: prompt, camera, movement, duration and order. Character: name, appearance, personality, visual and voice rules, sample dialogue and reference management. Asset: description, existing identity locks, review state and source preview. Reuse existing validation and ownership/revision checks; fetch authoritative data rather than treating the graph summary as the full record.

Direct field edits require Save/Cancel and show dirty/saving/saved/error state. Switching selection or route guards unsaved edits. A failed save retains the draft. Do not silently discard concurrent server updates; use existing version/revision contracts where present and surface conflicts.

Chat composer displays who/what the request targets and contextual example actions. Example chips fill the draft for review; they do not submit or charge. The selected record's current prompt is visible in Details/context summary, not automatically copied over the user's chat draft. AI changes remain proposals under existing Run controls; deterministic field edits save directly. Supported changes use existing actions; unsupported edits return an actionable limitation rather than claiming success.

Render jobs are status records. Outputs offer playback, downloads and the existing editor. The video editor edits supported caption/shot metadata; it does not edit rendered pixels. Composition timing/track forms remain the source of truth; no freeform HTML editor, new timeline engine or OpenCut integration in this redesign.

## Native engines and generation

Engine/session selector is visible in Studio and uses existing /director APIs. Distinguish CLI unavailable, installed but authentication unverified, no session, ready, running and review required. Create/select a project session inline; advanced history/recovery stays linked. No automatic native inference on page load or session creation, no silent engine fallback, no fabricated authentication success.

A compact next-step strip shows relevant prerequisites: session, story/scenes, character/reference readiness, keyframe review, render, voice/caption/edit. Each item links to the corresponding existing action. This is guidance derived from backend state, not a second workflow state machine.

Paid generation always remains deliberate: preserve reference approval, frozen identities and voices, explicit per-scene render, durable request/reconciliation behavior and no automatic paid retries. Show the action and affected item before Run. Display a price only when a reliable backend estimate exists; otherwise say paid generation without inventing a quote. Saving a prompt does not imply regenerating existing media.

## Visual and accessibility direction (builder proposal)

An operational creative workspace: quiet neutral surfaces, clear hierarchy, one restrained action accent, readable entity labels, consistent Inter typography and spacing. Keep light/dark/system support. Reduce decorative borders and redundant cards, reserve semantic colors for entity identity/status, use real previews when available. The redesign changes information architecture and interaction before decorative effects.

Visible labels and keyboard focus; aria-current navigation, labelled dialogs/tabs, announced save/error state, keyboard-accessible alternatives to graph gestures, 44px mobile targets, reduced-motion respect. Graph pan/zoom remains explicit, with useful fit and focus controls. Long names, translated text, large asset lists and portrait previews must not cover primary controls. English is the initial interface language, preserving user-authored multilingual content; no runtime translation dependency.

## Implementation constraints and builder calls

Use existing Svelte 5, SvelteKit, Tailwind, xyflow and component library. No new app dependencies. Refactor shared selection/workbench state and extract reusable controls where needed; remove superseded navigation/panel code only when callers migrate. Existing URLs/deep links, project boundaries, APIs, native session IDs and media data remain valid. Keep deployment changes out of scope. Builder may choose exact colors, spacing, component boundaries and breakpoints subject to these behavior/accessibility requirements.

## Verification and acceptance

Test real browser desktop and narrow mobile, including populated and empty project, missing backend and CLI, session create/select, every node kind, selection switch with dirty form accept/cancel, field save/error/conflict, draft preservation, contextual chat proposal target, stale proposal refusal, drawer/focus/keyboard, long names and overflow. Reuse and update existing component/native/canvas tests. Test rendering boundaries with mocks; no paid inference merely for UI QA. Run type checking and production build, relevant backend tests if contracts change. Record tested versus untested states; do not claim every possible combination.

Deliver working preview on the existing Tailscale link, screenshots, QA evidence and concise remaining limitations. Existing published download packages are not automatically rebuilt.
