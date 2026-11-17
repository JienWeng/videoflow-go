<script lang="ts">
 import {completeSceneRenders} from "$lib/production/completion.mjs";
  import {conversationTurns,conversationView} from './history.mjs';
  import {subscribeJobs} from '$lib/sse';
  import {runBackgroundOp} from '$lib/ops';
  import {workflowNext,storyReferencesApproved,hasPendingSceneReferences} from './workflow.mjs';
  import {protectDraft} from '$lib/protect-draft';
  import {storyCreation} from '$lib/create/story-creation.svelte';
  import { onMount, tick } from 'svelte';
  import { APIError, get, mediaUrl, post } from '$lib/api';
  import ActionCard from '$lib/chat/ActionCard.svelte';
  import { Button } from '$lib/components/ui/button';
  import * as Tooltip from '$lib/components/ui/tooltip';
  import * as Message from '$lib/components/ui/message';
  import * as Attachment from '$lib/components/ui/attachment';
  import * as Bubble from '$lib/components/ui/bubble';
  import * as MessageScroller from '$lib/components/ui/message-scroller';
  import * as InputGroup from '$lib/components/ui/input-group';
  import * as Trace from '$lib/components/ui/trace';
  import * as Marker from '$lib/components/ui/marker';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import ActivityView from './ActivityView.svelte';
  import { normalizeActivity } from './activity.mjs';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import LoaderCircle from '@lucide/svelte/icons/loader-circle';
  import ArrowUpRight from '@lucide/svelte/icons/arrow-up-right';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Eraser from '@lucide/svelte/icons/eraser';
  import MessageSquare from '@lucide/svelte/icons/message-square';
  import Maximize2 from '@lucide/svelte/icons/maximize-2';
  import Minimize2 from '@lucide/svelte/icons/minimize-2';
  import PanelRightClose from '@lucide/svelte/icons/panel-right-close';
  import X from '@lucide/svelte/icons/x';
  import PenLine from '@lucide/svelte/icons/pen-line';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';

  /**
   * Next-step chip: `send` posts the message immediately, `fill` pre-fills the
   * prompt input for the user to finish (e.g. refine instructions), `href`
   * links to a page.
   */
  interface Chip {
    label: string;
    send?: string;
    fill?: string;
    href?: string;
    focusId?:string;
  }

  /** In-chat progress row for a background op (storyboard/assets/shots/caption). */
  interface OpProgress {
    id: string;
    label: string;
    status: 'running' | 'done' | 'failed';
    /** Error or restore note shown under the row. */
    detail?: string;
  }

  interface Msg {
    role: 'user' | 'assistant';
    retry?: string;
    setupSceneId?:string;
    storyOperationId?:string;storyStage?:string;storyReady?:boolean;referenceIds?:string[];castIds?:string[];
    repairContext?: boolean;
    sessionReview?: boolean;
    activity?: unknown[];
    dismissed?: boolean;
 briefSuperseded?:boolean;
    text: string;
    intent?: any;
    options?: any;
    /** Prerequisite warnings from the backend, shown above the action card. */
    warnings?: string[];
    /** Project-state snapshot from the backend (drives the unknown card). */
    state?: any;
    /** Inline next-step suggestions rendered inside the bubble. */
    chips?: Chip[];
    /** Background-op progress row (spinner → check/cross). */
    op?: OpProgress;
    /** Generated-image thumbnails (storyboard / assets results). */
    thumbs?: { src: string; alt: string }[];
  }

  interface SelectedNode {
    id: string;
    kind: string;
    label: string;
  }

  let {
    onfocus,
    onguide,
    onmutate,
    onhide,
    onclear,
    onresize,
    embedded = false,
    expanded = false,
    selected = null
  }: {
    onfocus?: (id: string) => void;
    onguide?: (id:string)=>void;
    onmutate?: () => void | Promise<void>;
    onhide?: () => void;
    onclear?: () => void;
    onresize?: () => void;
    embedded?: boolean;
    expanded?: boolean;
    selected?: SelectedNode | null;
  } = $props();

  const greeting = (): Msg => ({
    role: 'assistant',
    text: 'Describe your story, or select an item on the canvas and tell me what to change. Review each proposal before running it.'
  });

  let messages = $state<Msg[]>([greeting()]);
  const isEmpty=$derived(messages.length===1&&messages[0].text===greeting().text);
  let input = $state('');
  protectDraft(()=>!!input.trim(),'You have an unsent message. Discard it and leave?');
  let busy = $state(false);
  let reviewMode=$state(false), capabilities=$state<any>(null), activeSessionId=$state('');
  let cancelling=$state(false), cancelError=$state('');
  async function stopPlanning(){if(!busy||!activeSessionId||cancelling)return;const generation=conversationGeneration;const id=activeSessionId;cancelling=true;cancelError='';try{await post(`/director/sessions/${id}/cancel`,{});}catch(e){if(generation===conversationGeneration){cancelling=false;cancelError=`Could not request cancellation: ${String(e)}`;}}}
  let checkpointPrompts=$state<string[]>([]);
  const pendingOps=$derived(messages.filter(m=>m.op?.status==='running'));
  function loadCheckpoints(key:string){try{const values=JSON.parse(sessionStorage.getItem(key+'.checkpoints')??'[]');checkpointPrompts=Array.isArray(values)?values.filter(v=>typeof v==='string').slice(-20):[];}catch{checkpointPrompts=[];}}
  function toggleCheckpoint(m:Msg){checkpointPrompts=checkpointPrompts.includes(m.text)?checkpointPrompts.filter(t=>t!==m.text):[...checkpointPrompts,m.text].slice(-20);const key=activeSessionId?'videoflow.chat.'+activeSessionId:storageKey;if(key)try{sessionStorage.setItem(key+'.checkpoints',JSON.stringify(checkpointPrompts))}catch{/* checkpoints remain usable for this visit */}}
  const specialistSupported=$derived(capabilities?.subagents?.supported===true);
  async function refreshCapabilities(){const generation=conversationGeneration;try{const result=await get('/chat/capabilities');if(generation!==conversationGeneration)return;capabilities=result;if(!result?.subagents?.supported)reviewMode=false;}catch{if(generation===conversationGeneration){capabilities=null;reviewMode=false;}}}
  function subagentEvents(list:any[]){return normalizeActivity((list??[]).map(a=>({kind:'subagent',id:a.id,title:a.name??a.role??'Specialist',status:a.status,detail:[a.summary,a.error].filter(Boolean).join('\n'),suggestions:a.suggestions})));}
  function messageActivity(m:Msg){return normalizeActivity([...(m.activity??[]),...(m.op?[{kind:'tool',id:m.op.id,title:m.op.label,status:m.op.status,detail:m.op.detail}]:[])]);}
  function reusePrompt(text:string){if(input.trim()&&!window.confirm('Replace your current prompt draft?'))return false;input=text;focusComposer();return true;}
  function prepareReviewChanges(m:Msg){const notes=normalizeActivity(m.activity).filter(e=>e.kind==='subagent'&&e.status==='done').map(e=>[e.title,e.detail,...e.suggestions].filter(Boolean).join('\n')).join('\n\n');if(notes&&reusePrompt('Prepare a proposal using this advisory review. Do not run changes yet.\n\n'+notes))reviewMode=false;}


  /**
   * Follow the user's language for our canned UI copy (hints, the new-story
   * seed): true when the most recent user turn contains CJK characters. The
   * project is bilingual, so this keeps prompts and hints in step with the
   * person typing. Defaults to Chinese — the seeded content is Chinese-first.
   */
  const userPrefersZh = $derived.by(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].role !== 'user') continue;
      return /[一-鿿]/.test(messages[i].text);
    }
    return false;
  });

  // ---------------------------------------------------------------------
  // Session persistence: messages survive navigating away from Studio.
  // Keyed per project; restore on mount (last 30), save on every change.
  // ---------------------------------------------------------------------
  let storageKey = $state<string | null>(null);
 let localSessionLabel = $state('Local sessions');

  /** Drop the non-serializable / refetched parts (state, callbacks live in
   * the template anyway — cards re-render fine from intent + options). */
  function serializableMsg(m: Msg) {
    const { role, text, intent, options, warnings, chips, op, thumbs, retry, activity, dismissed,briefSuperseded,repairContext,sessionReview,storyOperationId,storyStage,storyReady,referenceIds,castIds,setupSceneId } = m;
    return { role, text, intent, options, warnings, chips, op, thumbs, retry, activity:normalizeActivity(activity), dismissed,briefSuperseded,repairContext,sessionReview,storyOperationId,storyStage,storyReady,referenceIds,castIds,setupSceneId };
  }

  let conversationGeneration=0;
  let historyLoading=$state(true), historyError=$state(''), sessionRequired=$state(false);
  async function initPersistence() {
    const generation=++conversationGeneration;
    historyLoading=true;historyError='';sessionRequired=false;busy=false;storageKey=null;activeSessionId='';checkpointPrompts=[];cancelling=false;cancelError='';void refreshCapabilities();
    try {
      const status=await get('/director/status');if(generation!==conversationGeneration)return;
      if(status.all_text_local){
        const sessions=await get('/director/sessions');if(generation!==conversationGeneration)return;
        const session=sessions.find((item:any)=>item.selected);
        if(!session){sessionRequired=true;messages=[greeting()];return;}
        activeSessionId=session.id;loadCheckpoints('videoflow.chat.'+session.id);localSessionLabel=`${session.engine} · ${session.title}`;
        const turns=await get(`/director/sessions/${session.id}/turns`);if(generation!==conversationGeneration)return;
        const restored:Msg[]=[];
        for(const turn of conversationTurns(turns)){
          if(turn.request_json?.user_message)restored.push({role:'user',text:turn.request_json.user_message});
          if(turn.result_json?.reply)restored.push({role:'assistant',text:turn.result_json.reply,activity:subagentEvents(turn.result_json.subagents??[])});
          else if(turn.error)restored.push({role:'assistant',text:turn.error});
        }
        const key='videoflow.chat.'+session.id;
        let cached:unknown;try{cached=JSON.parse(sessionStorage.getItem(key)??'null')}catch{cached=null}
        messages=conversationView(restored,cached);if(!messages.length)messages=[greeting()];storageKey=key;return;
      }
      const project=await get('/projects/active');if(generation!==conversationGeneration)return;
      const key=`videoflow.chat.${project?.id??'default'}`;
      try{const saved=JSON.parse(sessionStorage.getItem(key)??'null');messages=Array.isArray(saved)&&saved.length?saved.slice(-30).map((m:Msg)=>m.op?.status==='running'?{...m,op:{...m.op,status:'failed',detail:'interrupted — check Activity'}}:m):[greeting()];}catch{messages=[greeting()];}
      storageKey=key;loadCheckpoints(key);
    }catch(e){if(generation===conversationGeneration)historyError=String(e);}
    finally{if(generation===conversationGeneration)historyLoading=false;}
  }

  $effect(() => {
    if (!storageKey) return;
    // JSON.stringify reads every nested prop → deep-tracks the messages.
    const json = JSON.stringify(messages.map(serializableMsg));
    try {
      sessionStorage.setItem(storageKey, json);
    } catch {
      /* quota — drop silently, chat still works in-memory */
    }
  });

  function clearChat() {
    messages = [greeting()];
    if (storageKey) {try{sessionStorage.removeItem(storageKey)}catch{/* in-memory clear still works */}}
  }

  /**
   * Called from the command palette ("New story"): seed the composer with a
   * fresh-story prompt in the user's language and focus it, so a new project
   * is one keystroke from a script. We pre-fill rather than auto-send — the
   * user still types the actual idea.
   */
  export function startNewStory() {
    input = userPrefersZh ? 'Write a new story: ' : 'Write a new story: ';
    inputWrapper?.querySelector('textarea')?.focus();
  }

  let graphData = $state<any>(null);
  let styleData = $state<any>(null);

  /**
   * One pass over /graph (scenes, shots, storyboards and render jobs are all
   * nodes) shared by the suggestion strip and the post-run follow-ups.
   */
  function analyzeGraph(graph: any) {
    const nodes: any[] = graph?.nodes ?? [];
    const edges: any[] = graph?.edges ?? [];
    const byId = new Map<string, any>(nodes.map((n) => [n.id, n]));

    const characters = nodes.filter((n) => n.type === 'character');
    const scenes = nodes.filter((n) => n.type === 'scene');
    const shots = nodes.filter((n) => n.type === 'shot');
    const outputs = nodes.filter((n) => n.type === 'output');

    const scenesWithShots = new Set(shots.map((sh) => sh.data?.scene_id).filter(Boolean));


    // Storyboard assets are linked to their scene by a metadata edge.
    const scenesWithStoryboard = new Set(
      edges
        .filter((e) => byId.get(e.target)?.data?.asset_type === 'storyboard')
        .map((e) => e.source)
    );

    const renderedScenes=completeSceneRenders(nodes.filter(n=>n.type==='render_job').map(n=>n.data??{}));

    /** The next pipeline action for a single scene, if it isn't done yet. */
    function sceneNextChip(sceneId: string, label: string): Chip | null {
      if (!scenesWithShots.has(sceneId))
        return { label: `Plan shots for ${label}`, focusId:sceneId, send: `Plan shots for ${label}` };
      if (!scenesWithStoryboard.has(sceneId))
        return { label: `Prepare keyframes for ${label}`, focusId:sceneId, send: `Prepare keyframes for ${label}` };
      if(hasPendingSceneReferences(graph,sceneId))return {label:`Replace excluded images for ${label}`,href:`/scenes?scene_id=${encodeURIComponent(sceneId)}`,focusId:sceneId};
      if (!renderedScenes.has(sceneId))
        return { label: `Render《${label}》`, focusId:sceneId, send: `render scene 《${label}》` };
      return null;
    }

    return {
      byId,
      characters,
      scenes,
      outputs,
      scenesWithShots,
      scenesWithStoryboard,
      renderedScenes,
      sceneNextChip
    };
  }

  /**
   * Suggest the next steps from the analyzed graph plus /style. When a canvas
   * node is selected, chips target that node instead.
   */
  function computeChips(graph: any, style: any, sel: SelectedNode | null): Chip[] {
    const {
      byId,
      characters,
      scenes,
      outputs,
      scenesWithShots,
      scenesWithStoryboard,
      renderedScenes,
      sceneNextChip
    } = analyzeGraph(graph);

    // Selection-aware chips replace the globals while a node is selected.
    if (sel) {
      const out: Chip[] = [];
      if (sel.kind === 'scene') {
        const next = sceneNextChip(sel.id, sel.label);
        if (next) out.push(next);
        out.push({ label: `Refine this scene…`, fill: `Refine this scene: ` });
      } else if (sel.kind === 'shot') {
        out.push({ label: 'Refine this shot…', fill: 'Refine this shot: ' });
        const sceneId = byId.get(sel.id)?.data?.scene_id;
        const scene = sceneId ? byId.get(sceneId) : null;
        if (scene) {
          const next = sceneNextChip(scene.id, scene.label);
          if (next) out.push(next);
        }
      } else if (sel.kind === 'output') {
        out.push({ label: 'Add captions to this video', send: 'Add captions to this video' });
        const qa = byId.get(sel.id)?.data?.qa_issues;
        if (Array.isArray(qa) && qa.length)
          out.push({ label: 'Fix the latest render', send: 'fix the latest render' });
      } else if (sel.kind === 'character') {
        out.push({
          label: `Write a story featuring ${sel.label}`,
          send: `Write a story featuring ${sel.label}`
        });
      }
      if (sel.kind === 'asset') return [{label:'Edit asset details',href:`/library?tab=media&asset_id=${encodeURIComponent(sel.id)}`}];
      if (sel.kind === 'render_job') return [{label:'View render progress',href:'/render'}];
      if (out.length) return out.slice(0, 3);
      // Unhandled kinds (asset, render_job) fall through to global chips.
    }

    const out: Chip[] = [];
    if (!characters.length) out.push({ label: 'Add characters first', href: '/characters' });
    if (!scenes.length) {
      out.push({ label: 'Write a story', send: 'Help me write a short story with one scene' });
    } else {
      if (!style?.style_prompt) {
        out.push({ label: 'Ingest style from story', send: 'ingest style from story' });
      }
      const noShots = scenes.find((s) => !scenesWithShots.has(s.id));
      if (noShots) out.push({ label: `Plan shots for ${noShots.label}`, send: `Plan shots for ${noShots.label}` });
      const noStoryboard = scenes.find(
        (s) => scenesWithShots.has(s.id) && !scenesWithStoryboard.has(s.id)
      );
      if (noStoryboard)
        out.push({ label: `Prepare keyframes for ${noStoryboard.label}`, send: `Prepare keyframes for ${noStoryboard.label}` });
      const notRendered = scenes.find(
        (s) => scenesWithStoryboard.has(s.id) && !renderedScenes.has(s.id)
      );
      if (notRendered)
        out.push({ label: `Render《${notRendered.label}》`, send: `render scene 《${notRendered.label}》` });

      // QA found issues on an output → offer a retry.
      if (outputs.some((o) => Array.isArray(o.data?.qa_issues) && o.data.qa_issues.length)) {
        out.push({ label: 'Fix the latest render', send: 'fix the latest render' });
      }
      // Latest output is missing burned-in captions.
      const latest = outputs[outputs.length - 1];
      if (latest && !latest.data?.captioned_path) {
        out.push({ label: 'Add captions to the latest video', send: 'Add captions to the latest video' });
      }
      if (scenes.every((s) => renderedScenes.has(s.id))) {
        out.push({ label: 'Start another story', fill: 'Write a new story: ' });
        out.push({ label: 'Suggest props', send: 'Suggest props' });
      }
    }
    return out.slice(0, 4);
  }

  const guidedSteps=$derived(workflowNext(graphData));
  let chips = $derived(computeChips(graphData, styleData, selected));

  async function refreshChips() {
    try {
      const [graph, style] = await Promise.all([get('/graph'), get('/style').catch(() => null)]);
      graphData = graph;
      await refreshStoryApprovals();
      styleData = style;
    } catch {
      graphData = null;
      styleData = null;
    }
  }

  let inputWrapper = $state<HTMLDivElement | null>(null);

  function applyChip(c: Chip) {
    if(c.focusId)(onguide??onfocus)?.(c.focusId);
    if (c.fill) {
      input = c.fill;
      inputWrapper?.querySelector('textarea')?.focus();
    } else if (c.send) {
      input = c.send;
      focusComposer();
    }
  }

  onMount(() => {
    refreshChips();
    void initPersistence().then(restoreGuidedOperation);
    const update=()=>{void initPersistence().then(restoreGuidedOperation)};
    const stop=subscribeJobs(()=>{void refreshChips()},()=>{void refreshChips()});
    window.addEventListener('videoflow-session-changed',update);
    window.addEventListener('focus',refreshChips);
    return ()=>{stop();window.removeEventListener('videoflow-session-changed',update);window.removeEventListener('focus',refreshChips)};
  });

  /** Context passed up by ActionCard after a successful run. */
  interface RunContext {
    action: string;
    sceneId?: string;
    characterId?:string;
    outputId?: string;
  }

  const captionsChip: Chip = { label: 'Add captions to the latest video', send: 'Add captions to the latest video' };

  // ---------------------------------------------------------------------
  // Background-op progress: when an ActionCard launches a background op we
  // append a Task progress row that flips to done/failed on the op result.
  //
  // Navigation policy: we NEVER goto() on the user's behalf mid-conversation
  // (a chip click like Add captions to this video must not yank them off the page).
  // Instead completed ops get prominent href chips — "Open in editor",
  // "View in Assets", "Track in Render" — which navigate normally when the
  // user clicks them (the one sanctioned navigation).
  // ---------------------------------------------------------------------

  /** Result handed to ActionCard so it can settle the progress row. */
  interface OpHandle {
    done: (op: { result_json?: Record<string, any> | null; output_id?: string | null }) => unknown;
    fail: (op: { error?: string | null }) => unknown;
  }

  /** Map generated asset ids to image thumbnails (cap 4, click → /assets). */
  async function assetThumbs(ids: string[]): Promise<{ src: string; alt: string }[]> {
    if (!ids.length) return [];
    try {
      const assets: any[] = await get('/assets');
      const byId = new Map(assets.map((a) => [a.id, a]));
      return ids
        .map((id) => byId.get(id))
        .map((a) => a && { src: mediaUrl(a.file_path), alt: a.name || 'generated asset' })
        .filter((t): t is { src: string; alt: string } => !!t?.src)
        .slice(0, 4);
    } catch {
      return [];
    }
  }

  /** Attach result thumbnails + onward-navigation chips to the op row. */
  async function attachOpResults(m: Msg | undefined, ctx: RunContext, result: any) {
    if (!m) return;
    const rj = result?.result_json ?? {};
    if(ctx.action==='generate_script'){
      m.storyOperationId=result.id??m.storyOperationId;
      m.storyStage=rj.status;m.castIds=rj.cast_character_ids??[];m.referenceIds=rj.story_asset_ids??[];
      m.text=rj.status==='awaiting_character_review'?'Your saved story can continue to storyboarding. Images do not need approval.':['awaiting_reference_review','ready_to_render'].includes(rj.status)?'Your story and images are ready. You can adjust them or render a scene.':'Story planning completed. Review the saved work before continuing.';
      m.chips=[{label:'Open story progress',href:`/create?op_id=${encodeURIComponent(m.storyOperationId??'')}`}];
      const scene=rj.scene_ids?.[0];if(scene)m.chips.push({label:'Show story on canvas',focusId:scene,fill:'What is the next step for this story?'});
      await refreshStoryApprovals();return;
    }
    if(ctx.action==='generate_character_reference'){
      m.text='Character reference generated and ready to use.';m.chips=[{label:'View character image',href:`/characters/${encodeURIComponent(ctx.characterId??'')}#np-character-references`,focusId:ctx.characterId}];return;
    }
    if (ctx.action === 'caption') {
      const outputId = rj.output_id ?? result?.output_id ?? ctx.outputId;
      if (outputId)
        m.chips = [{ label: 'Open in editor', href: `/editor/${outputId}` }];
    } else if (ctx.action === 'storyboard' || ctx.action === 'generate_assets' || ctx.action==='generate_image') {
      const ids: string[] = rj.asset_ids ?? (rj.asset_id ? [rj.asset_id] : []);
      const thumbs = await assetThumbs(ids);
      if (thumbs.length) m.thumbs = thumbs;
      m.chips = [{ label: 'View in Assets', href: '/assets' }];
    }
  }

  async function refreshStoryApprovals(){
    const pending=messages.filter(m=>m.storyStage==='awaiting_character_review');if(!pending.length)return;
    try{const generation=conversationGeneration;const [characters,assets,ops]=await Promise.all([get('/characters'),get('/assets'),Promise.all(pending.map(m=>m.storyOperationId?get(`/ops/${encodeURIComponent(m.storyOperationId)}`):Promise.resolve(null)))]);if(generation!==conversationGeneration)return;
      pending.forEach((m,i)=>{const op=ops[i],state=op?.result_json;
        if(state?.status==='awaiting_character_review'){m.referenceIds=state.story_asset_ids??[];m.castIds=state.cast_character_ids??[]}
        if(op?.status==='succeeded'&&state?.status==='awaiting_reference_review'){m.storyStage=state.status;m.text='Your story and images are ready. You can adjust them or render a scene.';return}
        m.storyReady=op?.status!=='running'&&storyReferencesApproved(m.castIds??[],m.referenceIds??[],characters,assets);
      });
    }catch{for(const m of pending)m.storyReady=false}
  }
  async function restoreGuidedOperation(){const generation=conversationGeneration;try{const project=await get('/projects/active');const ops=await get(`/ops?project_id=${encodeURIComponent(project.id)}&kind=video_generation&limit=1`);if(generation!==conversationGeneration)return;const op=ops?.[0];if(op?.status!=='succeeded'||!['awaiting_character_review','awaiting_reference_review','ready_to_render'].includes(op.result_json?.status)||messages.some(m=>m.storyOperationId===op.id))return;const m:Msg={role:'assistant',text:''};messages.push(m);await attachOpResults(messages[messages.length-1],{action:'generate_script'},op)}catch{/* Regular chat stays usable if progress cannot be restored. */}}
  let continuingStory=$state(false);
  async function continueStory(m:Msg){
    if(!m.storyOperationId||continuingStory||!m.storyReady)return;
    continuingStory=true;const generation=conversationGeneration;
    m.op={id:m.storyOperationId,label:'Storyboard images',status:'running'};
    const settle=()=>{continuingStory=false};
    try{await runBackgroundOp(`/videos/generate/${m.storyOperationId}/continue`,{}, {label:'Storyboard images',onDone:async op=>{settle();if(generation!==conversationGeneration)return;if(m.op)m.op.status='done';await attachOpResults(m,{action:'generate_script'},op);await onmutate?.();await refreshChips()},onFail:op=>{settle();if(generation!==conversationGeneration)return;if(m.op){m.op.status='failed';m.op.detail=op.error??'Storyboarding needs attention.'}m.text=op.error??'Storyboarding needs attention. Review the existing work before retrying.'}})}catch(e){settle();if(generation===conversationGeneration){if(m.op)m.op.status='failed';m.text=String(e)}}
  }

  const missingVoiceCharacter=(text:string)=>text.match(/\/voices\?character_id=([a-zA-Z0-9_-]+)/)?.[1]??text.match(/\bspeaker\s+(char_[a-zA-Z0-9_-]+)\b.*voice/i)?.[1];
  async function openSceneSetup(m:Msg){
    try{
      const scenes=await get('/scenes');const char=missingVoiceCharacter(m.text);
      const choices=m.setupSceneId?scenes.filter((s:any)=>s.id===m.setupSceneId):char?scenes.filter((s:any)=>{let cast=s.character_ids_json??[];if(typeof cast==='string'){try{cast=JSON.parse(cast)}catch{cast=[]}}return cast.includes(char)}):scenes;
      if(!choices.length){messages.push({role:'assistant',text:'That scene or speaker is no longer in this project. Select the current scene to prepare its setup.'});return}
      messages.push({role:'assistant',text:'Choose native sound, saved character voices or silence below. Existing ready voices are kept. Saving options does not generate media; the render action follows the displayed paid-call plan.',intent:{action:'render_scene',scene_id:choices.length===1?choices[0].id:''},options:{scenes:choices.map((s:any)=>({id:s.id,title:s.title})),characters:[],outputs:[],caption_styles:[]}});
    }catch(e){messages.push({role:'assistant',text:'Could not load scene setup. '+String(e)})}
  }

  /** ActionCard launched a background op: append the in-chat progress row. */
  function handleOpStart(label: string, ctx: RunContext): OpHandle {
    const id = crypto.randomUUID();
    messages.push({ role: 'assistant', text: '', op: { id, label, status: 'running' } });
    const find = () => messages.find((m) => m.op?.id === id);
    return {
      done: async (op) => {
        const m = find();
        if (m?.op) m.op.status = 'done';
        await attachOpResults(m, ctx, op);
      },
      fail: (op) => {
        const m = find();
        if (!m?.op) return;
        m.op.status = 'failed';
        m.op.detail = op.error ?? 'failed';
        m.text = op.error ?? 'This action could not finish. Try its action button again; existing work is kept.';
        if(ctx.action==='render_scene')m.setupSceneId=ctx.sceneId;
      }
    };
  }

  /**
   * Compose the assistant follow-up posted after a card runs successfully.
   * Deterministic: derived from the *refreshed* graph, so the chips reflect
   * the state the action just produced. Returns null when there is nothing
   * useful to propose (e.g. unknown actions).
   */
  function buildFollowUp(ctx: RunContext): Msg | null {
    if (!ctx.action || ctx.action === 'unknown') return null;
    const g = analyzeGraph(graphData);

    /** Wrap-up message when a scene's pipeline has nothing left to do. */
    function sceneComplete(title: string): Msg {
      return {
        role: 'assistant',
        text: `《${title}》 is fully rendered.`,
        chips: [captionsChip, { label: '写一个新故事…', fill: 'Write a new story: ' }]
      };
    }

    switch (ctx.action) {
      case 'generate_script': {
        const chips = g.scenes
          .map((s) => g.sceneNextChip(s.id, s.label))
          .filter((c): c is Chip => !!c)
          .slice(0, 2);
        return {
          role: 'assistant',
          text: 'Script created. Next: expand a scene.',
          chips
        };
      }
      case 'generate_scenes':
      case 'generate_shots':
      case 'storyboard':
      case 'generate_assets':
      case 'refine_scene':
      case 'refine_shot': {
        if (!ctx.sceneId) return null;
        const title = g.byId.get(ctx.sceneId)?.label ?? 'this scene';
        const next = g.sceneNextChip(ctx.sceneId, title);
        if (!next) return sceneComplete(title);
        return {
          role: 'assistant',
          text: `Done — next for 《${title}》:`,
          chips: [next, { label: `改进场景《${title}》…`, fill: `改进场景《${title}》：` }]
        };
      }
      case 'render_scene':
        return {role:'assistant',text:'Video plan submitted. Native sound follows the saved audio mode; saved character voices use the separate speech workflow. Once clips finish, assemble them, add music and time captions from the actual audio.',chips:[{label:'Track in Videos',href:'/render'},{label:'Assemble & add music',href:ctx.sceneId?`/compositions?scene_id=${encodeURIComponent(ctx.sceneId)}`:'/compositions'}]};
      case 'render_shot':
      case 'retry_render':
        return {
          role: 'assistant',
          text: 'Render submitted — when it succeeds, captions are one click:',
          chips: [
            { label: 'Track in Render — 查看渲染', href: '/render' },
            captionsChip
          ]
        };
      case 'caption':
        return {
          role: 'assistant',
          text: 'Captions added.',
          chips: computeChips(graphData, styleData, null).slice(0, 2)
        };
      case 'plan_assets':
        return {
          role: 'assistant',
          text: 'Asset suggestions are on the card above. When you are ready:',
          chips: computeChips(graphData, styleData, null).slice(0, 2)
        };
      case 'style_ingest':
        return {
          role: 'assistant',
          text: 'Style ingested from the story. Next:',
          chips: computeChips(graphData, styleData, null).slice(0, 2)
        };
      case 'delete_scene':
        return {
          role: 'assistant',
          text: 'Scene deleted. Next:',
          chips: computeChips(graphData, styleData, null).slice(0, 2)
        };
      default:
        return null;
    }
  }

  /** Card ran successfully: refresh, then keep the conversation moving. */
  async function handleRan(_result: any, ctx?: RunContext) {
    await onmutate?.();
    await refreshChips();
    if (!ctx) return;
    if(ctx.action==='generate_script'||ctx.action==='generate_character_reference')return;
    const follow = buildFollowUp(ctx);
    if (follow) messages.push(follow);
  }

  function intentSummary(intent: any, options: any): string {
    const lines: string[] = [`Proposed action: ${String(intent.action).replaceAll('_',' ')}`];
    if (intent.scene_id) {
      const s = options?.scenes?.find((x: any) => x.id === intent.scene_id);
      lines.push(`Scene: ${s?.title ?? 'Selected scene'}`);
    }
    if (intent.character_id) {
      const c = options?.characters?.find((x: any) => x.id === intent.character_id);
      lines.push(
        `Character: ${c?.name ?? 'Selected character'}`
      );
    }
    if (intent.shot_id) lines.push('Shot: selected shot');
    if (intent.output_id) {
      const o = options?.outputs?.find((x: any) => x.id === intent.output_id);
      const file = o?.video?.split('/').pop();
      lines.push(`Output: ${file??'Selected output'}`);
    }
    if (intent.style) lines.push(`style: ${intent.style}`);
    if (intent.language) lines.push(`language: ${intent.language}`);
    if (intent.idea) lines.push(`idea: ${intent.idea}`);
    if (intent.scene_count != null) lines.push(`scenes: ${intent.scene_count}`);
    if (intent.confidence != null) lines.push(`confidence: ${intent.confidence}`);
    return lines.join('\n\n');
  }

  /** Return keyboard focus to the composer (scoped: card textareas live
   * outside `inputWrapper`, so this can't grab one of them). */
  async function focusComposer() {
    await tick();
    inputWrapper?.querySelector('textarea')?.focus();
  }

  let repairingContext=$state(false);
  async function repairContext(message:Msg){if(!activeSessionId||repairingContext)return;repairingContext=true;const generation=conversationGeneration;try{const result=await post(`/director/sessions/${activeSessionId}/repair-context`,{});if(generation!==conversationGeneration)return;message.repairContext=false;message.sessionReview=!!result.review_required;message.text=result.review_required?'Unavailable references removed. Review the interrupted AI session before sending again.':'Unavailable references removed. Your message is ready to retry.';reusePrompt(message.retry??'');void refreshCapabilities();}catch(e){if(generation===conversationGeneration)message.text=`Could not repair context. ${String(e)}`;}finally{repairingContext=false}}
 function activeCreativeBrief(){
  for(let i=messages.length-1;i>=0;i--){const m=messages[i];if(m.intent?.action!=='generate_script'||m.dismissed)continue;
   if(messages.slice(i+1).some(next=>next.op?.label==='Story & reference images'&&next.op.status!=='failed'))return undefined;
   return m.intent.creation_brief??(m.intent.idea?{idea:m.intent.idea}:undefined);
  }
 }
  async function send(text?: string) {
    const message = (text ?? input).trim();
    if (!message || busy || pendingOps.length || historyLoading || sessionRequired || historyError) return;
    const generation=conversationGeneration;
    input = '';
    if(isEmpty)messages=[];
    // Conversation memory: the last 6 turns (text only — no cards/state).
    const history = messages.slice(-6).map((m) => ({ role: m.role, text: m.text }));
    messages.push({ role: 'user', text: message });
    busy = true;cancelling=false;cancelError='';
    const useSpecialists=reviewMode&&specialistSupported;
    const reviewIndex=messages.length;
    if(useSpecialists)messages.push({role:'assistant',text:'Specialist review requested. No story changes will run.',activity:normalizeActivity((capabilities.subagents.roles??[]).map((role:any)=>({kind:'subagent',title:role.name,status:'pending'})))});
    try {
      const r = await post(useSpecialists?'/chat/review':'/chat', { message, ...(useSpecialists?{}:{history,creation_brief:activeCreativeBrief()}), selected: selected ? {id:selected.id,kind:selected.kind} : null });
      if(generation!==conversationGeneration)return;
      if(useSpecialists){messages[reviewIndex]={role:'assistant',text:r.reply,activity:subagentEvents(r.subagents??[])};void refreshCapabilities();return;}
      const intent = { ...r.intent };
      if(intent.action==='generate_script'){for(const m of messages)if(m.intent?.action==='generate_script'&&!m.dismissed){m.dismissed=true;m.briefSuperseded=true;}}
      messages.push({
        role: 'assistant',
        text: intent.reply,
        intent,
        options: r.options,
        warnings: r.warnings ?? [],
        state: r.state,
        activity: normalizeActivity(r.activity)
      });
    } catch (e) {
      if(generation!==conversationGeneration)return;
      if(useSpecialists){messages.splice(reviewIndex,1);void refreshCapabilities();}
      messages.push({
        role: 'assistant',
        text: `Could not send your message. ${(e as Error).message}`,
        retry: message,
        repairContext:e instanceof APIError&&e.code==='chat_context_invalid',
        sessionReview:e instanceof APIError&&e.code==='chat_session_review_required'
      });
    } finally {
      if(generation===conversationGeneration){busy = false;
      refreshChips();
      focusComposer();}
    }
  }
</script>

{#snippet chipRow(list: Chip[])}
  {#each list as c (c.label)}
    {#if c.href}
      <!-- Navigation chip: solid, with the leaving-arrow affordance. -->
      <Button
        variant="secondary"
        size="sm"
        href={c.href}
        class="min-h-9 h-auto whitespace-normal text-left shrink-0 rounded-full px-3 text-xs font-normal"
      >
        {c.label}
        <ArrowUpRight class="size-3" />
      </Button>
      {#if c.focusId}<Button variant="ghost" size="sm" aria-label={'Show on canvas: '+c.label} onclick={()=>(onguide??onfocus)?.(c.focusId!)}>Show on canvas</Button>{/if}
    {:else if c.fill}
      <!-- Pre-fill chip: outline + dashed + pen, so it reads as "edit then send"
           rather than "run now" (the solid send chips below). -->
      <Button
        variant="outline"
        size="sm"
        class="min-h-9 h-auto whitespace-normal text-left shrink-0 rounded-full border-dashed px-3 text-xs font-normal text-muted-foreground"
        disabled={busy}
        onclick={() => applyChip(c)}
      >
        <PenLine class="size-3" />
        {c.label}
      </Button>
    {:else}
      <!-- Send chip: solid, runs immediately on click. -->
      <Button
        variant="secondary"
        size="sm"
        class="min-h-9 h-auto whitespace-normal text-left shrink-0 rounded-full px-3 text-xs font-normal"
        disabled={busy}
        onclick={() => applyChip(c)}
      >
        {c.label}
      </Button>
    {/if}
  {/each}
{/snippet}



<div class="h-full flex flex-col">
  {#if !embedded}<div class="flex items-center gap-2 border-b border-border px-4 py-2">
    {#if !embedded}
    <MessageSquare class="size-4 shrink-0 text-muted-foreground" />
    <div class="min-w-0">
      <div class="text-sm font-semibold leading-tight">Chat</div>
      <div class="truncate text-[11px] leading-tight text-muted-foreground">
        <a href="/director" class="underline" title="Choose native engine, saved session and reference context">{localSessionLabel}</a>
      </div>
    </div>
    {/if}
    <div class="ml-auto flex items-center gap-1">
      {#if !embedded}
      <Button
        variant="ghost"
        size="icon"
        class="size-7 text-muted-foreground"
        aria-label={expanded ? 'Restore chat panel size' : 'Expand chat panel'}
        title={expanded ? 'Restore chat panel size' : 'Expand chat panel'}
        onclick={onresize}
      >
        {#if expanded}<Minimize2 class="size-3.5" />{:else}<Maximize2 class="size-3.5" />{/if}
      </Button>
      <Button
        variant="ghost"
        size="icon"
        class="size-7 text-muted-foreground"
        aria-label="Hide chat panel"
        title="Hide chat panel"
        onclick={onhide}
      >
        <PanelRightClose class="size-3.5" />
      </Button>
      {/if}
      <Tooltip.Provider delayDuration={300}>
        <Tooltip.Root>
          <Tooltip.Trigger>
            {#snippet child({ props })}
              <Button
                {...props}
                variant="ghost"
                size="icon"
                class="size-7 text-muted-foreground"
                aria-label="Clear chat"
                onclick={clearChat}
              >
                <Eraser class="size-3.5" />
              </Button>
            {/snippet}
          </Tooltip.Trigger>
          <Tooltip.Content side="bottom">
            Clear chat — history is kept for this session
          </Tooltip.Content>
        </Tooltip.Root>
      </Tooltip.Provider>
    </div>
  </div>

  {/if}
  <MessageScroller.Provider autoScroll>
  <MessageScroller.Root class="flex-1 min-h-0">
    <MessageScroller.Viewport role="log" aria-label="Story conversation" class="motion-reduce:scroll-auto">
    <MessageScroller.Content class="gap-5 px-4 pt-5 pb-4">
      {#if pendingOps.length}<ActivityView events={[{kind:'queue',title:'Work in progress',status:'running',items:pendingOps.map(m=>({title:m.op!.label,status:'running'}))}]} />{/if}
      {#if historyLoading}<div role="status" class="flex items-center gap-2 text-sm text-muted-foreground"><LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />Loading conversation…</div>
      {:else if historyError}<div role="alert" class="space-y-3 py-4"><h3 class="font-semibold">Could not load conversation</h3><p class="text-sm text-muted-foreground">Your saved history has not been replaced. {historyError}</p><Button variant="outline" onclick={initPersistence}>Retry conversation</Button></div>
      {:else if sessionRequired}<div class="space-y-3 py-4"><MessageSquare class="size-7 text-muted-foreground" /><h3 class="font-semibold">Connect your story assistant</h3><p class="text-sm text-muted-foreground">Choose a session above, or set up Codex or Claude Code to start planning.</p><Button variant="outline" href="/director">Set up AI session</Button></div>
      {:else if isEmpty}
        <div class="flex flex-col gap-4 py-3">
          <h3 class="text-lg font-semibold">{selected?`Let’s work on ${selected.label}`:'Start with an idea'}</h3>
          <p class="text-sm leading-relaxed text-muted-foreground">{selected?'Describe a change. You’ll review it before it runs.':'What story would you like to tell?'}</p>
          <div class="flex flex-wrap gap-2" aria-label="Conversation starters">
            {#each (selected?[{title:'Suggest improvements',prompt:`Review ${selected.label} and suggest improvements. Do not run changes yet.`}]:[{title:'Build a story',prompt:'Help me plan a story about '},{title:'Plan the storyboard',prompt:'Help me plan storyboard shots for '},{title:'Develop a character',prompt:'Help me develop a character who '}]) as starter}
              <button type="button" class="min-h-10 rounded-lg border border-border px-3 py-2 text-sm hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring" onclick={()=>reusePrompt(starter.prompt)}>{starter.title}</button>
            {/each}
          </div>
          {#if !selected&&!embedded}<Button variant="outline" class="self-start" onclick={()=>storyCreation.show()}>Create story</Button>{/if}
        </div>
      {:else}
      {#each messages as m, i (i)}
        <MessageScroller.Item messageId={'turn-'+i}>
        <Message.Root align={m.role==='user'?'end':'start'}>
          <Message.Content>
            <Message.Header class="text-xs font-medium text-muted-foreground">{m.role === 'user' ? 'You' : 'Story assistant'}</Message.Header>

            {#if m.text}
              <Bubble.Root align={m.role==='user'?'end':'start'} variant={m.role==='user'?'muted':'ghost'}><Bubble.Content class="whitespace-pre-wrap break-words leading-relaxed">{m.text}</Bubble.Content></Bubble.Root>
              {#if m.role==='assistant'&&(m.setupSceneId||missingVoiceCharacter(m.text))}<Button variant="outline" size="sm" disabled={busy} onclick={()=>openSceneSetup(m)}>Set up scene voices here</Button>{/if}
            {/if}
            {#if m.intent?.action==='unknown' && m.state?.next_steps?.length}<ol class="list-decimal space-y-1 pl-5 text-sm">{#each m.state.next_steps as step}<li>{step}</li>{/each}</ol>{/if}
            {#if m.repairContext}<Button variant="outline" size="sm" class="mt-2" disabled={busy||repairingContext} onclick={()=>repairContext(m)}>{repairingContext?'Repairing…':'Repair chat context'}</Button>{/if}
            {#if m.sessionReview}<Button variant="outline" size="sm" class="mt-2" href="/director">Review AI session</Button>{/if}
            {#if m.storyStage==='awaiting_character_review'}<div class="space-y-2"><p class="text-xs text-muted-foreground">{m.storyReady?'References ready. Continue to scene images.':'Replace missing or excluded images before continuing.'}</p><Button size="sm" disabled={!m.storyReady||continuingStory||busy} onclick={()=>continueStory(m)}>{continuingStory?'Creating storyboard images…':'Continue storyboarding'}</Button></div>{/if}
            {#if m.retry&&!m.repairContext&&!m.sessionReview}<Button variant="outline" size="sm" class="mt-2" disabled={busy} onclick={()=>reusePrompt(m.retry??'')}>Edit and retry</Button>{/if}
            <ActivityView events={messageActivity(m)} />
            {#if normalizeActivity(m.activity).some(e=>e.kind==='subagent'&&e.status==='done')}<Button variant="outline" size="sm" class="self-start" disabled={busy} onclick={()=>prepareReviewChanges(m)}>Prepare changes from review</Button>{/if}
            {#if m.thumbs?.length}
              <div class="mt-2 flex flex-wrap gap-1.5">
                {#each m.thumbs as t (t.src)}
                  <a href="/library?tab=media" title="{t.alt} — view in Media">
                    <img
                      src={t.src}
                      alt={t.alt}
                      loading="lazy"
                      class="size-16 rounded-md border border-border object-cover transition-opacity hover:opacity-80"
                    />
                  </a>
                {/each}
              </div>
            {/if}
            {#if m.chips?.length}
              <div class="mt-2 flex flex-wrap items-center gap-1.5">
                {@render chipRow(m.chips)}
              </div>
            {/if}
            {#if m.warnings?.length}
              <div class="mt-2 space-y-1">
                {#each m.warnings as w (w)}
                  <p class="flex items-start gap-1.5 text-amber-500 text-xs">
                    <TriangleAlert class="size-3.5 shrink-0 mt-0.5" />
                    <span>{w}</span>
                  </p>
                {/each}
              </div>
            {/if}
            {#if m.intent && m.intent.action !== 'unknown' && !m.dismissed}
              <ActionCard
                intent={m.intent}
 onbriefchange={brief=>m.intent={...m.intent,creation_brief:brief,idea:brief.idea}}
                options={m.options}
                projectState={m.state}
                onfocus={onguide??onfocus}
                onran={handleRan}
                onop={handleOpStart}
                onsuggest={(text) => { input=text; focusComposer(); }}
              />
              <Button variant="ghost" size="sm" class="self-start" onclick={()=>m.dismissed=true}>Dismiss proposal</Button>
            {:else if m.dismissed}<p class="text-xs text-muted-foreground">{m.briefSuperseded?'Earlier creative brief · updated below':'Proposal dismissed from this view.'}</p>{/if}
            {#if m.role==='user'}<Message.Footer><Button size="sm" variant="ghost" disabled={busy} onclick={()=>reusePrompt(m.text)}>Reuse prompt</Button><Button size="sm" variant="ghost" disabled={busy} aria-pressed={checkpointPrompts.includes(m.text)} onclick={()=>toggleCheckpoint(m)}>{checkpointPrompts.includes(m.text)?'Remove prompt checkpoint':'Save prompt checkpoint'}</Button></Message.Footer>{/if}
          </Message.Content>
        </Message.Root>
        {#if m.role==='user'&&checkpointPrompts.includes(m.text)}<Marker.Root variant="separator"><Marker.Content class="flex flex-wrap items-center gap-2">Prompt checkpoint<Button variant="outline" size="sm" onclick={()=>reusePrompt(m.text)}>Use prompt</Button><span class="text-xs text-muted-foreground">Restores text only; saved work and conversation stay unchanged.</span></Marker.Content></Marker.Root>{/if}
        </MessageScroller.Item>
      {/each}
      {/if}
      {#if guidedSteps.length&&!busy}<section aria-label="Suggested next steps" class="space-y-2 border-t pt-3"><h4 class="text-sm font-medium">Next step</h4><div class="flex flex-wrap gap-2">{@render chipRow(guidedSteps)}</div><p class="text-xs text-muted-foreground">Suggestions focus the canvas and prepare your request. Review the proposal before running it.</p></section>{/if}
      {#if busy&&!reviewMode}
        <MessageScroller.Item messageId="working"><Message.Root><Message.Content><div role="status" class="flex items-center gap-2 text-sm text-muted-foreground"><LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />Working on your story…</div></Message.Content></Message.Root></MessageScroller.Item>
      {/if}
    </MessageScroller.Content>
    </MessageScroller.Viewport>
    <MessageScroller.Button aria-label="Jump to latest message" class="motion-reduce:transition-none" />
  </MessageScroller.Root>
  </MessageScroller.Provider>

  <div class="p-3 pt-2" bind:this={inputWrapper}>
    {#if selected||!isEmpty}<div class="mb-2 flex min-w-0 items-center justify-between gap-2">
      {#if selected}<Attachment.Root size="sm" class="min-w-0 max-w-full"><Attachment.Content class="min-w-0"><Attachment.Title class="truncate" title={selected.label}>{selected.kind.replace('_',' ')}: {selected.label}</Attachment.Title></Attachment.Content>{#if onclear}<Attachment.Actions><Attachment.Action aria-label="Clear selection" class="size-9 shrink-0" onclick={onclear}><X class="size-3.5" /></Attachment.Action></Attachment.Actions>{/if}</Attachment.Root>{:else}<span></span>{/if}
      {#if embedded&&!isEmpty}<Button variant="ghost" size="icon" class="size-9 shrink-0 text-muted-foreground" aria-label="Clear chat" title="Clear visible chat; saved session history is kept" disabled={busy||historyLoading||isEmpty} onclick={clearChat}><Eraser class="size-4" /></Button>{/if}
    </div>{/if}
    {#if !guidedSteps.length&&chips.length&&(!isEmpty||selected)}<div aria-label="Suggested prompts" class="mb-2 flex flex-wrap items-center gap-2">{@render chipRow(chips)}</div>{/if}
    <form onsubmit={e=>{e.preventDefault();void send();}}>
      <InputGroup.Root class="rounded-2xl bg-background shadow-none has-disabled:opacity-100 has-disabled:bg-background dark:has-disabled:bg-background">
        <InputGroup.Textarea aria-label="Message story assistant" bind:value={input} class="min-h-24 max-h-40 px-3 pt-3" placeholder={selected ? `What should change in ${selected.label}?` : 'Tell me your idea…'} onkeydown={e=>{if(e.key==='Enter'&&!e.shiftKey&&!e.isComposing){e.preventDefault();void send();}}} />
        <InputGroup.Addon align="block-end" class="gap-2">
          <DropdownMenu.Root><DropdownMenu.Trigger>{#snippet child({props})}<InputGroup.Button {...props} type="button" size="sm" aria-label="Assistant mode" disabled={busy}>{reviewMode?'Review':'Chat'}<ChevronDown class="size-3.5" /></InputGroup.Button>{/snippet}</DropdownMenu.Trigger>
            <DropdownMenu.Content align="start" side="top" class="w-64 max-w-[calc(100vw-2rem)]">
              <DropdownMenu.RadioGroup value={reviewMode?'specialists':'assistant'} onValueChange={value=>reviewMode=value==='specialists'}>
                <DropdownMenu.RadioItem value="assistant">Chat</DropdownMenu.RadioItem>
                <DropdownMenu.RadioItem value="specialists" disabled={!specialistSupported}>Specialist review{#if !specialistSupported}<span class="ml-auto text-xs text-muted-foreground">Unavailable</span>{/if}</DropdownMenu.RadioItem>
              </DropdownMenu.RadioGroup>
              <DropdownMenu.Separator />
              <DropdownMenu.Item>{#snippet child({props})}<a {...props} href="/director">AI session settings</a>{/snippet}</DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Root>
          <!-- Stop is offered only when the backend has a concrete local session to cancel. -->
          {#if busy&&activeSessionId}<InputGroup.Button type="button" size="sm" variant="outline" class="ml-auto" aria-label="Stop planning" disabled={cancelling} onclick={stopPlanning}>{cancelling?'Stopping…':'Stop'}</InputGroup.Button>{:else}<InputGroup.Button type="submit" size="icon-sm" variant="default" class="ml-auto" aria-label="Send message" disabled={busy||pendingOps.length>0||historyLoading||sessionRequired||!!historyError||!input.trim()}>{#if busy}<LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />{:else}<ArrowUp class="size-4" />{/if}</InputGroup.Button>{/if}
        </InputGroup.Addon>
      </InputGroup.Root>
    </form>
    {#if cancelError}<p role="alert" class="mt-2 text-xs text-destructive">{cancelError}</p>{/if}
    <p class="mt-2 px-1 text-[11px] text-muted-foreground">Enter to send · Shift + Enter for a new line</p>
  </div>
</div>
