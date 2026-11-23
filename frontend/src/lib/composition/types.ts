/** The editor's composition manifest (internal/composition.Manifest). */

export type ClipAdjustments = {
  fit: string;
  scale: number;
  x: number;
  y: number;
  brightness: number;
  contrast: number;
  saturation: number;
  opacity: number;
};

export type OverlayBox = { x: number; y: number; width: number; height: number };

export type Clip = {
  id: string;
  asset_id: string;
  start: number;
  trim: number;
  duration: number;
  speed: number;
  overlay?: OverlayBox | null;
  volume?: number | null;
  fade_in?: number;
  fade_out?: number;
  muted?: boolean;
  adjustments?: ClipAdjustments | null;
  has_audio?: boolean;
  sha256?: string;
  path?: string;
  original_duration?: number;
  width?: number;
  height?: number;
};

export type Cue = {
  id: string;
  utterance_id: string;
  start: number;
  end: number;
  text: string;
  character_id?: string;
  language?: string;
  source_clip_id?: string;
  /** How the time was found: when the words are heard in the clip, estimated per
   * shot, or when a voice-over line speaks it. */
  timing?: 'heard' | 'estimated' | 'voice';
  /** The script's wording when `text` shows what was actually said instead. */
  scripted?: string;
};

export type TextLayer = {
  id: string;
  text: string;
  start: number;
  end: number;
  size: number;
  color: string;
  background: string;
  font: string;
  position: string;
};

export type CompositionManifest = {
  version: number;
  width: number;
  height: number;
  fps: number;
  duration: number;
  caption_style: string;
  clips: Clip[];
  audio: Clip[];
  captions: Cue[];
  texts?: TextLayer[];
  caption_variants?: { language: string; captions: Cue[] }[];
  renderer?: string;
  revision?: string;
  caption_bindings?: unknown[];
};

/** A media item the editor can place (compositionSource), plus details GET /compositions/sources/{id} adds. */
export type CompositionSource = {
  id: string;
  name: string;
  type: string;
  media_kind: string;
  url: string;
  duration?: number;
  width?: number;
  height?: number;
  has_audio?: boolean;
};

/** GET /compositions/sources/{id} */
export type CompositionSourceInfo = {
  source: CompositionSource;
  duration: number;
  width: number;
  height: number;
  has_audio: boolean;
};

/** GET /compositions/runtime */
export type CompositionRuntime = {
  ready: boolean;
  version: string;
  source_commit: string;
  message: string;
  opencut?: string;
};

/** One saved edit in GET /compositions. */
export type SavedComposition = {
  revision: string;
  created_at: string;
  duration: number;
  clip_count: number;
  caption_count: number;
};

/** A spoken line VoiceoverPanel hands the editor to place on the audio track. */
export type VoiceLine = {
  asset_id: string;
  start: number;
  duration: number;
  /** The whole voice file; `duration` may stop a few milliseconds short to fit. */
  original_duration?: number;
  text: string;
  speaker: string;
  cached: boolean;
  /** "heard" when placed where the character says the line in the clip. */
  timing?: string;
  slot?: number;
  tempo?: number;
};

/** How voice lines treat the voices a clip was made with. */
export type VoiceSound = {
  mode: 'keep' | 'replace';
  /** Clips that speak with their own voices, and those to mute. */
  native: string[];
  mute: string[];
  lowerMusic: boolean;
};

/** POST /compositions/voiceover/status */
export type VoiceoverStatus = {
  native: { clip_id: string; output_id: string; check: 'passed' | 'flagged' | 'kept' | 'unchecked'; finding?: string }[];
  verified: number;
  flagged: number;
  unchecked: number;
  recommended: 'keep' | 'replace';
};

/** POST /compositions/voiceover */
export type VoiceoverResult = {
  items?: VoiceLine[];
  skipped?: { reason: string }[];
  generated: number;
  reused: number;
  shifted: number;
  overrun: number;
  mode?: 'keep' | 'replace';
  native_clip_ids?: string[];
  mute_clip_ids?: string[];
  kept_native?: number;
  too_long?: { text: string; speaker: string; start: number; duration: number; slot: number; tempo_needed: number; fits_with_speed_up: boolean }[];
  sped_up?: number;
  speed_up_limit?: number;
};
