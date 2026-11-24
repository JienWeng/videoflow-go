export interface AguiToolCall {
  name: string;
  /** The JSON arguments as streamed so far. */
  args: string;
  /** TOOL_CALL_RESULT's content, once the tool call has a result. */
  result?: string;
}

/** What a run has shown so far. */
export interface AguiView {
  status: 'idle' | 'running' | 'finished' | 'error';
  messageId?: string;
  text: string;
  steps: { name: string; status: 'running' | 'done' }[];
  toolCalls: Record<string, AguiToolCall>;
  state: any;
  result?: any;
  error?: { message: string; code?: string };
}

export interface AguiMessage {
  id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  toolCallId?: string;
}

/** AG-UI RunAgentInput. */
export interface AguiRunInput {
  threadId: string;
  runId: string;
  state: unknown;
  messages: AguiMessage[];
  tools: unknown[];
  context: { description: string; value: string }[];
  forwardedProps?: unknown;
}

export class AguiUnavailable extends Error {
  constructor(message: string);
}
export class AguiRunError extends Error {
  constructor(message: string, code?: string);
  code?: string;
  readonly status: number;
}
export function createSSEParser(onEvent: (event: any) => void): { push(text: string): void; end(): void };
export function applyPatch<T = any>(doc: T, ops: { op: string; path: string; value?: unknown; from?: string }[]): T;
export function reduceEvent(view: AguiView, event: any): AguiView;
export function emptyView(): AguiView;
export function runAgent(options: {
  url: string;
  input: AguiRunInput;
  onView?: (view: AguiView, event: any) => void;
  signal?: AbortSignal;
  fetch?: typeof fetch;
}): Promise<any>;
export const STEP_LABELS: Readonly<Record<'read_project' | 'ask_ai_session' | 'check_answer', string>>;
export function stepsActivity(steps: AguiView['steps']): {
  kind: 'plan';
  id: string;
  title: string;
  status: 'running' | 'done';
  items: { title: string; status: 'running' | 'done' }[];
}[];
