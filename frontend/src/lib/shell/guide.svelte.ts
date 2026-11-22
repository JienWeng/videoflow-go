/**
 * App-wide guidance state: the Guide panel (checklist + tips) and the
 * step-by-step tour. Singleton like palette.svelte.ts so any page can open it.
 */
const store = $state({ panelOpen: false, touring: false });

export const guide = {
  get panelOpen() {
    return store.panelOpen;
  },
  set panelOpen(v: boolean) {
    store.panelOpen = v;
  },
  get touring() {
    return store.touring;
  },
  openPanel() {
    store.panelOpen = true;
  },
  startTour() {
    store.panelOpen = false;
    store.touring = true;
  },
  endTour() {
    store.touring = false;
  }
};

export type TourStep = {
  target: string; // value of data-tour on the element to highlight
  title: string;
  body: string;
  view?: 'studio' | 'story'; // Studio view the target lives in
  openAssistant?: boolean;
};

// Plain-language walkthrough of the main flow, in the order people use it.
export const TOUR_STEPS: TourStep[] = [
  {
    target: 'views',
    title: 'Three ways to work',
    body: 'Canvas shows your whole project as a map. Story lists scenes one by one with their progress. Edit video is where you put clips together, add music and captions, and export.',
    view: 'story'
  },
  {
    target: 'progress',
    title: 'Always know the next step',
    body: 'This bar shows how many scenes have video and the single most useful next step. Click it to jump to that scene.',
    view: 'story'
  },
  {
    target: 'scene-next',
    title: 'Each scene tells you what it needs',
    body: 'Shots → reference images → storyboard → video. The button opens the right section; anything that costs credits asks you to confirm first.',
    view: 'story'
  },
  {
    target: 'assistant',
    title: 'Ask the assistant anything',
    body: 'Describe what you want in plain words, like “make the opening scene brighter” or “why did this scene fail?”. It prepares a button for you to review before anything runs.',
    view: 'studio',
    openAssistant: true
  },
  {
    target: 'create-story',
    title: 'Start a new story',
    body: 'Create story walks you through an idea, style, length and characters, then plans the scenes for you.',
    view: 'studio'
  },
  {
    target: 'nav-videos',
    title: 'Finished videos',
    body: 'Videos shows every render, its progress, and anything that needs attention after a provider problem.'
  },
  {
    target: 'nav-activity',
    title: 'What is running',
    body: 'Activity lists background work — storyboards, renders, exports — and explains failures in plain words.'
  },
  {
    target: 'guide',
    title: 'Help is always here',
    body: 'Open Guide any time for your checklist, this tour, and tips. Settings is where you connect your AI and media providers.'
  }
];
