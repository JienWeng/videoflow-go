export type ActivityStatus = 'pending' | 'running' | 'done' | 'failed' | 'skipped';
export type ActivityKind = 'progress' | 'reasoning' | 'plan' | 'queue' | 'task' | 'tool' | 'terminal' | 'stack-trace' | 'test-results' | 'subagent' | 'sources' | 'inline-citation' | 'file-tree' | 'context';
export interface ActivityFile { name: string; path?: string; type: 'file' | 'folder'; children: ActivityFile[] }
export interface ActivityEvent {
 id?: string; kind: ActivityKind; title: string; status?: ActivityStatus; detail?: string; code?: string;
 items: {title: string; status?: ActivityStatus; detail?: string}[];
 sources: {title: string; url: string; description?: string}[];
 files: ActivityFile[]; suggestions: string[]; children: ActivityEvent[];
 usage?: {used: number; limit: number; input?: number; output?: number; reasoning?: number};
}
export const ACTIVITY_LIMITS: Readonly<{events:number;depth:number;text:number;title:number;entries:number;totalText:number}>;
export function normalizeActivity(input: unknown): ActivityEvent[];
