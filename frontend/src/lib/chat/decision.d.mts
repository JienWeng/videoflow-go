export interface DecisionOption { label: string; value: string }
export interface ChatDecision {
  mode: 'act' | 'ask' | 'inform';
  assumptions: string[];
  question: string;
  options: DecisionOption[];
  key?: string;
  overruled?: string;
}
export interface DecisionAnswer { key?: string; question: string; label: string; value: string }
export function normalizeDecision(raw: unknown): ChatDecision;
export function assumptionLine(raw: unknown): string;
export function isAsking(raw: unknown): boolean;
export function answerChips(raw: unknown): { label: string; send: string; answer: DecisionAnswer }[];
export function rejectionKey(intent: { action?: string; scene_id?: string | null; character_id?: string | null; output_id?: string | null; script_id?: string | null; scene_ids?: string[] } | undefined | null): string;
export function decisionLabel(key: string, named?: { id: string; name?: string; title?: string }[]): string;
