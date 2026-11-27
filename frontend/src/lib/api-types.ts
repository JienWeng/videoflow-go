/**
 * Shapes of the Go backend's JSON responses (see internal/models and internal/api).
 * Fields the UI does not rely on being present are optional.
 */

/** A picture's review state, stored on assets as metadata_json.consistency_review. */
export type ConsistencyReview = string;

export type ProjectCounts = { scenes: number; characters: number; assets: number };

export type Project = {
  id: string;
  name: string;
  description: string;
  cover?: string;
  is_active: boolean;
  counts?: ProjectCounts;
  created_at: string;
  updated_at: string;
};

export type Character = {
  id: string;
  project_id?: string | null;
  name: string;
  description: string;
  appearance: string;
  personality: string;
  visual_rules_json?: string[] | null;
  voice_rules_json?: string[] | null;
  sample_dialogue: string;
  reference_asset_ids_json?: string[] | null;
  created_at: string;
  updated_at: string;
};

export type AssetMetadata = {
  [key: string]: unknown;
  consistency_review?: ConsistencyReview;
  locked_traits?: Record<string, string>;
  variant_group_id?: string;
  scene_id?: string;
  director_context_revision?: string;
  composition_revision?: string;
};

export type Asset = {
  id: string;
  project_id?: string | null;
  type: string;
  name: string;
  file_path: string;
  tags_json?: string[] | null;
  description: string;
  character_id?: string | null;
  metadata_json?: AssetMetadata | null;
  created_at: string;
  updated_at: string;
};

export type Script = {
  id: string;
  project_id?: string | null;
  idea: string;
  title: string;
  summary: string;
  draft_json?: unknown;
  created_at: string;
  updated_at: string;
};

export type SceneSpec = {
  [key: string]: unknown;
  story_asset_ids?: string[];
  generation_brief?: { [key: string]: unknown; style?: string };
};

export type Scene = {
  id: string;
  project_id?: string | null;
  script_id?: string | null;
  scene_order: number;
  title: string;
  summary: string;
  duration: number;
  aspect_ratio: string;
  character_ids_json?: string[] | null;
  asset_ids_json?: string[] | null;
  scene_json?: SceneSpec | null;
  created_at: string;
  updated_at: string;
};

export type Shot = {
  id: string;
  scene_id: string;
  shot_order: number;
  duration: number;
  prompt: string;
  camera: string | null;
  movement: string | null;
  asset_ids_json?: string[] | null;
  shot_json?: Record<string, unknown> | null;
  created_at: string;
  updated_at: string;
};

export type RenderJob = {
  id: string;
  project_id?: string | null;
  scene_id?: string | null;
  shot_id?: string | null;
  provider: string;
  provider_endpoint?: string;
  model: string;
  provider_job_id?: string | null;
  status: string;
  stage?: string | null;
  progress?: string | null;
  request_json?: Record<string, unknown> | null;
  response_json?: unknown;
  error?: string | null;
  created_at: string;
  updated_at: string;
};

export type RenderOutput = {
  id: string;
  render_job_id: string;
  video_path: string;
  thumbnail_path?: string | null;
  captioned_path?: string | null;
  captions_json?: CaptionSegment[] | null;
  score?: number | null;
  qa_json?: unknown;
  selected: boolean;
  notes: string;
  created_at: string;
  updated_at: string;
};

export type StyleGuide = {
  id: string;
  project_id?: string | null;
  name: string;
  style_prompt: string;
  palette: string;
  lighting: string;
  audience: string;
  tone: string;
  created_at: string;
  updated_at: string;
};

export type CaptionSegment = {
  id?: string;
  character_id?: string;
  language?: string;
  start: number;
  end: number;
  text: string;
};

/** Response of POST endpoints that start a background op (`?background=true`). */
export type OpStarted = { op_id: string };

export type SceneProgress = {
  scene_id: string;
  title: string;
  order: number;
  stage: string;
  label: string;
  detail: string;
  shots: number;
  step: number;
  next: { action: string; label: string };
  blocked: boolean;
  video: { state: string; reason?: string; job_id?: string; service?: string };
  reference_mode?: boolean;
};

export type ProjectProgress = {
  scenes: SceneProgress[];
  total: number;
  video_ready: number;
  attention: number;
  summary: string;
  next?: SceneProgress | null;
};

/** GET /scenes/{id}/references and the attach/detach replies. */
export type SceneReferenceState = {
  scene_id: string;
  title: string;
  updated_at: string;
  asset_ids: string[];
  references: Asset[];
  storyboards: Asset[];
  missing_asset_ids: string[];
  missing_character_ids: string[];
  references_ready: boolean;
  storyboard_ready: boolean;
  active_storyboard_id: string;
  scene_input_revision: string;
  context_revision: string;
};

export type PlannedAsset = {
  name: string;
  type?: string;
  description: string;
  asset_id?: string;
  existing_asset_id?: string;
  reuse: boolean;
  reason?: string;
  asset_type: string;
  shot_orders?: number[];
};

/** POST /scenes/{id}/assets/plan */
export type AssetPlan = {
  scene_id?: string;
  assets: PlannedAsset[];
  suggested_shots?: string[];
  reasoning?: string;
};

/** An op's result_json: its shape depends on the op kind, so every field is optional. */
export type OpResult = {
  [key: string]: unknown;
  status?: string;
  stage?: string;
  stages?: string[];
  reply?: string;
  subagents?: unknown[];
  scene_id?: string;
  scene_ids?: string[];
  asset_id?: string;
  asset_ids?: string[];
  job_id?: string;
  character_references?: boolean;
  script_id?: string;
  cast_character_ids?: string[];
  story_asset_ids?: string[];
  evidence?: string;
  videos_only?: boolean;
  scene_titles?: string[];
  video_url?: string;
  /** Set when a video run stopped on a scene the video service never answered about. */
  stuck?: { job_id: string; scene_id?: string; service: string; message: string };
  /** POST /characters/reference-sheets: the characters drawn and those that failed. */
  made?: CharacterSheetOutcome[];
  failed?: CharacterSheetOutcome[];
  image_calls?: number;
  /** Full auto's self-check (internal/api/selfcheck_policy.go): what it flagged, and for
   * a flagged clip the question to ask (needs_review with a chat decision). */
  self_check?: SelfCheckFlag[];
  needs_review?: boolean;
  decision?: unknown;
  /** Autopilot: what the run did, in words, and which clips it approved or left. */
  summary?: string;
  approval?: { approved: { label: string }[]; skipped: { label: string; reason: string }[] };
};

/** A picture or clip the self-check flagged (kind clip_audio: the speech check). */
export type SelfCheckFlag = {
  kind: string;
  id: string;
  label: string;
  scene_id?: string;
  verdict: string;
  score: number;
  change?: string;
  problem?: string;
};

/** One character in a picture batch (internal/api/character_batch.go). */
export type CharacterSheetOutcome = {
  character_id: string;
  name: string;
  reference_asset_ids?: string[];
  error?: string;
};

/** GET /characters/reference-sheets: what a picture batch would draw, before anything is spent. */
export type CharacterSheetPlan = {
  characters: CharacterSheetOutcome[];
  image_calls: number;
  ready: boolean;
  problem?: string;
};

/**
 * GET /{scenes|shots}/{id}/revisions. models.Revision sends entity_type, entity_id and
 * snapshot_json and created_at.
 */
export type EditRevision = {
  id: string;
  entity_type?: string;
  entity_id?: string;
  snapshot_json?: unknown;
  created_at: string;
};

/** A canvas node's data from GET /graph (internal/db/graph.go); fields depend on the node type. */
export type GraphNodeData = {
  [key: string]: unknown;
  file_path?: string;
  asset_type?: string;
  consistency_review?: string;
  reference_asset_ids_json?: string[] | null;
  aspect_ratio?: string;
  duration?: number;
  summary?: string;
  scene_id?: string | null;
  shot_order?: number;
  prompt?: string;
  camera?: string | null;
  movement?: string | null;
  status?: string;
  video_path?: string;
  thumbnail_path?: string | null;
  captioned_path?: string | null;
  scene_titles?: string[];
  created_at?: string;
};

export type StyleGuidance = { story?: string; shots?: string; image?: string; video?: string };

/** One look in GET /style-presets (internal/presets.Preset plus a fingerprint). */
export type StylePreset = {
  id: string;
  name: string;
  description: string;
  style_prompt?: string;
  guidance?: StyleGuidance;
  fingerprint?: string;
};

export type StorytellingPreset = { id: string; name: string; description: string; guidance: string };

export type PresetPack = {
  format: string;
  schema_version: number;
  id: string;
  name: string;
  version: string;
  author?: string;
  license?: string;
  presets: StylePreset[];
};

/** The project's own look as GET /style-presets lists it. */
export type ProjectStylePreset = { id: string; name: string; description: string; fingerprint: string };

/** The editable fields of a project style (a StyleGuide without its ids and dates). */
export type StyleFields = Pick<StyleGuide, 'name' | 'style_prompt' | 'palette' | 'lighting' | 'tone' | 'audience'>;

/** GET /style-presets */
export type StyleCatalog = {
  has_project_style: boolean;
  project_style: ProjectStylePreset | null;
  presets: StylePreset[];
  packs: { pack: PresetPack; hash: string }[];
  built_in_count: number;
  storytelling_presets: StorytellingPreset[];
};

/** A setup step that blocks generation (generationPrerequisites). */
export type MissingPrerequisite = { route: string; setting: string; reason: string };

/** GET /videos/preflight */
export type VideoPreflight = {
  ready: boolean;
  automatic_assets?: boolean;
  character_references?: boolean;
  native_images?: boolean;
  effective_style?: string;
  has_project_style?: boolean;
  video_durations?: number[] | null;
  supports_references?: boolean;
  missing: MissingPrerequisite[];
  warnings?: { reason: string }[];
  effective_routes: { video_provider: string; image_provider: string; video_model: string; image_model: string };
  style?: { selected: string; aspect: string };
};

/** result_json of a video_generation op (storyContinuation in internal/api). */
export type StoryGenerationResult = {
  project_id?: string;
  script_id?: string;
  scene_ids?: string[];
  stages?: string[];
  created_character_ids?: string[];
  cast_character_ids?: string[];
  character_reference_ids?: string[];
  character_references?: boolean;
  story_assets?: PlannedAsset[];
  automatic_assets?: boolean;
  story_asset_ids?: string[];
  status: string;
  storyboards_made?: number;
  render_job_ids?: string[];
};

/** GET /render-jobs/{id} */
export type RenderJobDetail = { job: RenderJob; outputs: RenderOutput[] };

/** A local AI conversation (db.DirectorSession). */
export type DirectorSession = {
  id: string;
  project_id: string;
  engine: string;
  native_id: string;
  title: string;
  status: string;
  selected: boolean;
  created_at: string;
  model?: string;
  effort?: string;
};

/** One recorded request in a director session (db.DirectorTurn). */
export type DirectorTurn = {
  id: string;
  session_id: string;
  kind: string;
  status: string;
  request_json?: { user_message?: string; audio_mode?: string; [key: string]: unknown } | null;
  result_json?: OpResult | null;
  error: string;
  created_at: string;
};

/** GET /director/status */
export type DirectorStatus = {
  all_text_local: boolean;
  engines: { engine: string; installed: boolean; signed_in: boolean; method?: string; reason?: string }[];
  voice_default?: string;
  host_requirement?: string;
};

/** POST /director/sessions/ensure */
export type EnsuredSession = { session: DirectorSession | null; switched_from: string };

/** GET /costs/estimate */
export type CostEstimate = {
  currency: string;
  total: number;
  seconds: number;
  model: string;
  known: boolean;
  scenes?: {
    scene_id: string;
    title: string;
    seconds: number;
    clips: number;
    amount: number;
    note?: string;
    retry_seconds?: number;
    retry_amount?: number;
  }[];
  /** full_auto=true: the worst case, each scene that speaks made once more by the
   * automatic speech retry. */
  retry_seconds?: number;
  retry_amount?: number;
  worst_seconds?: number;
  worst_total?: number;
  retry_note?: string;
};

/** One service selling a video model (videoService in internal/api). */
export type VideoService = {
  provider: string;
  label: string;
  model: string;
  per_second: number;
  total: number;
  checked: string;
  connected: boolean;
  usable: boolean;
  checked_at?: string;
  problem?: string;
  cheapest: boolean;
  in_use: boolean;
  note?: string;
  key_check?: { state: string; message?: string };
};

/** GET /video-services and PUT /video-services/routing */
export type VideoServicesState = {
  families: { id: string; label: string }[];
  model: string;
  resolution: string;
  seconds: number;
  routing: string;
  current: { provider: string; label: string; model: string; family: string };
  services: VideoService[];
};

/** A provider's saved-key check (keyCheckView). */
export type KeyCheck = { state: string; message?: string; checked_at?: string; [key: string]: unknown };

/** One entry of GET /settings/providers. */
export type ProviderInfo = {
  name: string;
  label: string;
  protocol: string;
  configured: boolean;
  models: string[];
  suggested_models: string[];
  default_model: string;
  allow_custom: boolean;
  from_db: boolean;
  key_check?: KeyCheck | null;
};

/** GET /assets/{id}/usage */
export type AssetUsage = {
  asset_id: string;
  uses: { kind: string; id: string; name: string }[];
  can_delete: boolean;
  review_stale?: boolean;
};

/** A speech estimate's verdict (internal/speech): fits, fits with little room, or cannot be said in the time. */
export type SpeechVerdict = 'ok' | 'tight' | 'too_long' | '';

/** One spoken line in a scene's speech budget (speechLine in internal/api/speech_budget.go). */
export type SpeechLine = {
  id: string;
  character_id: string;
  speaker: string;
  language: string;
  text: string;
  shot_id: string;
  /** line: its own shot; clip: the clip chosen in video options; auto: placed by time; scene: no shots yet */
  placed: 'line' | 'clip' | 'auto' | 'scene';
  start_hint?: number;
  seconds: number;
  start: number;
  end: number;
  verdict: SpeechVerdict;
};

/** GET/POST /scenes/{id}/speech-fit */
export type SceneSpeech = {
  scene_id: string;
  title: string;
  language: string;
  lines: number;
  verdict: SpeechVerdict;
  summary: string;
  shots: {
    shot_id: string;
    number: number;
    start: number;
    seconds: number;
    used: number;
    free: number;
    verdict: SpeechVerdict;
    budget: { unit: string; low: number; high: number; max: number };
    lines: SpeechLine[];
  }[];
  clips: {
    index: number;
    shot_ids: string[];
    seconds: number;
    used: number;
    free: number;
    verdict: SpeechVerdict;
    shots_over: number;
    summary: string;
    /** Seconds the words take, the room kept for music and ambience, and whether it is left. */
    speech_seconds?: number;
    breathing_room?: number;
    room_for_music?: boolean;
    room?: string;
  }[];
};

/** A spoken line as saved on a scene (SpokenLine in internal/api/caption_script.go). */
export type SavedSpokenLine = {
  id: string;
  character_id: string;
  language: string;
  text: string;
  visible: boolean;
  shot_id?: string;
  start_hint?: number;
};

/** POST /scenes/{id}/spoken-lines/fit: the scene's lines fitted to its shots, not saved. */
export type ScriptFit = {
  scene_id: string;
  expected_updated_at: string;
  before: SavedSpokenLine[];
  after: SavedSpokenLine[];
  before_fit: SceneSpeech;
  after_fit: SceneSpeech;
};

/** POST /outputs/{id}/speech-repair (speechRepair in internal/api/speech_repair.go):
 * the scene's lines before and after (only the flagged clip's lines differ), what the
 * speech check heard, and the price of making the clip again. Nothing is saved. */
export type SpeechRepair = {
  output_id: string;
  scene_id: string;
  scene_title: string;
  expected_updated_at: string;
  heard: string[];
  speech_end?: number;
  clip_length: number;
  line_ids: string[];
  before: SavedSpokenLine[];
  after: SavedSpokenLine[];
  before_fit: SceneSpeech;
  after_fit: SceneSpeech;
  quote: { plan_id: string; calls: number; seconds: number; storyboards: number; blocked?: string };
};

/** A scene's video settings (productionSettings in internal/api/production_plan.go). */
export type ProductionSettings = {
  dialogue_shots?: Record<string, string>;
  audio_mode: string;
  strategy: string;
  music_prompt: string;
  video_input?: string;
};

/** GET/PUT /scenes/{id}/production */
export type ProductionPlan = {
  id: string;
  scene_id: string;
  updated_at: string;
  resolution: string;
  settings: ProductionSettings;
  provider: string;
  model: string;
  audio_supported: boolean;
  lines: { id: string; text: string; speaker: string; shot_id: string; from_script?: boolean }[];
  units: {
    count: number;
    index: number;
    shot_ids: string[];
    start: number;
    length: number;
    duration: number;
    prompt: string;
    key: string;
    job_id?: string;
    status?: string;
  }[];
  calls: number;
  reused: number;
  generated_seconds: number;
  new_seconds: number;
  edited_seconds: number;
  storyboard_ready: boolean;
  references_ready: boolean;
  blocked?: string;
  /** Whether the spoken lines can be said in each clip; absent when nothing is said. */
  speech?: SceneSpeech;
  video_input: string;
  pictures?: { asset_id: string; name: string; kind: string; sheet?: boolean; character_id?: string }[];
};
