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
}
export function readSelfCheck(raw: unknown): SelfCheck | null;
export function needsAttention(c: SelfCheck | null): boolean;
export function selfCheckBadge(c: SelfCheck | null): { label: string; tone: 'ok' | 'warn' | 'bad' | 'muted' };
export function findingLines(c: SelfCheck | null): string[];
export function selfCheckKindFor(assetType: string | undefined | null): '' | 'character_sheet' | 'scene_picture';
