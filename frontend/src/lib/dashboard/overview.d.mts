export type OverviewItem = { id: string; label: string; detail: string; href: string; action: string; urgent: boolean };
export type OverviewAction = { id: string; label: string; detail: string; href: string };
export type RecentItem = { id: string; title: string; kind: string; href: string; status: string; time: number; date: string };
export function makeOverview(snapshot: { compositions?: any[] | null; graph: any; assets: any[]; ops: any[]; jobs: any[]; sessions: any[] | null; aiHealth: any; appHealth: any }, now?: number): {
  metrics: { scenes: number; characters: number; assets: number; videos: number };
  storyEmpty: boolean;
  primary: OverviewAction;
  attention: OverviewItem[];
  recent: RecentItem[];
  runningCount: number;
};
