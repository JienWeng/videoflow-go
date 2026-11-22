import type { Wand2 } from '@lucide/svelte';
import type { Scene } from '$lib/api-types';

/** One pipeline stage of a scene, as StoryWorkspace computes it for the stepper. */
export type SceneStep = {
  key: string;
  label: string;
  icon: typeof Wand2;
  done: boolean;
  busy: boolean;
  busyLabel: string;
  action: () => void;
  signal?: AbortSignal;
};

/** A scene plus the confirmed production plan SceneDetail attaches before rendering. */
export type SceneRenderRequest = Scene & {
  production_plan_id?: string;
  new_version?: boolean;
  confirmed_calls?: number;
  confirmed_seconds?: number;
  confirmed_storyboard_calls?: number;
};
