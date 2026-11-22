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
  text: string;
  speaker: string;
  cached: boolean;
};

/** POST /compositions/voiceover */
export type VoiceoverResult = {
  items?: VoiceLine[];
  skipped?: { reason: string }[];
  generated: number;
  reused: number;
  shifted: number;
  overrun: number;
};
