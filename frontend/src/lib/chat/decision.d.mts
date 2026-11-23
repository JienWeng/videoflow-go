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
