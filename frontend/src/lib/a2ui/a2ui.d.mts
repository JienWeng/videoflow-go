/** One A2UI surface as the client holds it. */
export interface Surface {
  id: string;
  catalogId: string;
  components: Record<string, A2UIComponent>;
  data: any;
}

export interface A2UIComponent {
  id: string;
  component: string;
  [prop: string]: any;
}

export const A2UI_VERSION: '1.0';
export const REPLY_ACTION: 'videoflow.reply';
export const CARD_RESULT_ACTION: 'videoflow.card_result';
export const CARD_ID: 'card';

export class A2UIUnavailable extends Error {
  constructor(message: string);
}
export class A2UIRunError extends Error {
  constructor(message: string, code?: string);
  code?: string;
  readonly status: number;
}
export function createSSEParser(onMessage: (message: any) => void): { push(text: string): void; end(): void };
export function getAt(data: any, pointer: string): any;
export function setAt(data: any, pointer: string, value: any): any;
export function applyMessage(surfaces: Record<string, Surface>, message: any): Record<string, Surface>;
export function resolve(value: any, data: any, scope?: any): any;
export function childList(children: any, data: any): [string, any][];
export function actionEvent(a: {
  name: string;
  surfaceId: string;
  sourceComponentId: string;
  context?: Record<string, unknown>;
}): { version: '1.0'; action: Record<string, unknown> };
export function runA2UI(options: {
  url: string;
  body: object;
  surfaces?: Record<string, Surface>;
  onUpdate?: (surface: Surface, message: any) => void;
  signal?: AbortSignal;
  fetch?: typeof fetch;
}): Promise<{ surface: Surface; result: any }>;
export function stepsActivity(steps: { name: string; label?: string; status: 'running' | 'done' }[] | undefined): {
  kind: 'plan';
  id: string;
  title: string;
  status: 'running' | 'done';
  items: { title: string; status: 'running' | 'done' }[];
}[];
export function localSurface(id: string, intent: any): Surface;
