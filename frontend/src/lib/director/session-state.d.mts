export type SessionStatus = { engine: string; status: string; native_id?: string };
export type AIHealth = { all_text_local?: boolean; engines?: { engine: string; installed: boolean }[] };
export function sessionState(session: SessionStatus | undefined, health: AIHealth | null): {
  label: string;
  help: string;
  canContinue: boolean;
  needsSetup: boolean;
};
