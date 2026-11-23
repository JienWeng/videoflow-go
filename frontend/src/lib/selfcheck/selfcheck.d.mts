export interface SelfCheckIssue { code: string; severity: 'low' | 'medium' | 'high'; detail: string }
export interface SelfCheck {
  verdict: 'pass' | 'fix' | 'fail';
  score: number;
  issues: SelfCheckIssue[];
  suggestion: string;
  change: string;
  note: string;
  unavailable: boolean;
  accepted: boolean;
  attempt: number;
  checkedAt: string;
  engine: string;
  /** The speech check's lines as heard (kind clip_audio only). */
  audio: SpeechCheck | null;
}
export interface HeardLine {
  text: string;
  heard: string;
  similarity: number;
  start: number | null;
  end: number | null;
  status: 'ok' | 'changed' | 'missing' | 'cut_off';
}
export interface SpeechCheck { lines: HeardLine[]; heardLanguage: string; expectedLanguage: string; cost: number }
export function readSelfCheck(raw: unknown, key?: string): SelfCheck | null;
export function checkKey(kind: string): 'self_check' | 'audio_check';
export function checkName(kind: string): string;
export function engineName(engine: string): string;
export function heardLines(c: SelfCheck | null): string[];
export function needsAttention(c: SelfCheck | null): boolean;
export function selfCheckBadge(c: SelfCheck | null): { label: string; tone: 'ok' | 'warn' | 'bad' | 'muted' };
export function findingLines(c: SelfCheck | null): string[];
export function selfCheckKindFor(assetType: string | undefined | null): '' | 'character_sheet' | 'scene_picture';
