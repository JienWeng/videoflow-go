/** Shapes of the chat endpoints (POST /chat, /chat/review, GET /chat/capabilities). */

/** A scene, character or output the action card can target. */
export interface ChatOption {
  id: string;
  title?: string;
  name?: string;
  video?: string | null;
  summary?: string;
  duration?: number;
  description?: string;
  appearance?: string;
  personality?: string;
}

export interface ChatOptions {
  scenes: ChatOption[];
  characters: ChatOption[];
  outputs: ChatOption[];
}

/** A video service the assistant offers to switch to (switch_video_service). */
export interface VideoSwitchChoice {
  mode: 'manual' | 'cheapest';
  model: string;
  provider: string;
  label: string;
  per_second: number;
}

/**
 * The assistant's proposed action (agents.ChatIntentResponse plus the fields
 * internal/api/chat.go adds). Cards built in the browser carry only some of them.
 */
export interface ChatIntent {
  action: string;
  reply?: string;
  editor_panel?: string;
  character_changes?: Record<string, string>;
  creation_brief?: Record<string, unknown>;
  asset_type?: string;
  director_proposal?: string;
  scene_id?: string | null;
  character_id?: string | null;
  cast_ids?: string[];
  shot_id?: string | null;
  output_id?: string | null;
  language?: string | null;
  idea?: string | null;
  style?: string;
  scene_count?: number | null;
  confidence?: number;
  script_id?: string;
  scene_ids?: string[];
  scenes?: { id: string; title: string; duration?: number }[];
  job_id?: string;
  scene_title?: string;
  service?: string;
  waiting_reason?: string;
  video_switch?: VideoSwitchChoice;
}

/** Backend project-state snapshot attached to the chat reply. */
export interface ChatProjectState {
  characters: number;
  has_style: boolean;
  scripts: number;
  scenes: {
    id: string;
    title: string;
    expanded: boolean;
    has_shots: boolean;
    has_storyboard: boolean;
    rendered: boolean;
  }[];
  next_steps: string[];
}

/** POST /chat */
export interface ChatReply {
  intent: ChatIntent & { reply: string };
  options: ChatOptions;
  state: ChatProjectState;
  warnings?: string[];
  activity?: unknown;
  session_id?: string;
  context_revision?: string;
}

/** One specialist's answer in POST /chat/review (specialistResult). */
export interface SpecialistResult {
  id: string;
  role: string;
  name: string;
  status: string;
  summary: string;
  suggestions: string[];
  error?: string;
}

/** POST /chat/review */
export interface ChatReview {
  reply: string;
  subagents?: SpecialistResult[];
}

/** GET /chat/capabilities */
export interface ChatCapabilities {
  subagents: {
    supported: boolean;
    engine?: string;
    mode?: string;
    roles?: { id: string; name: string }[];
    reason?: string;
  };
}

/** What StuckVideo reports once a waiting video request is settled. */
export interface StuckCheck {
  job_id: string;
  scene_id?: string;
  service: string;
  outcome: 'found' | 'failed' | 'unknown';
  message: string;
}

/** Passed up with a card's result so the chat can propose the next step. */
export interface RunContext {
  action: string;
  characterId?: string;
  sceneId?: string;
  outputId?: string;
}
