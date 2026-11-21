/** Pure non-destructive timeline edits; source media is never rewritten. */
export const defaults=()=>({fit:'contain',scale:1,x:50,y:50,brightness:1,contrast:1,saturation:1,opacity:1});
/** @param {any} clip @param {number} time @param {string} id */
export function splitClip(clip,time,id){if(!Number.isFinite(time)||time<=clip.start+.001||time>=clip.start+clip.duration-.001)throw Error('Place the playhead inside the selected clip to split.');const offset=time-clip.start;return [{...structuredClone(clip),duration:offset},{...structuredClone(clip),id,start:time,trim:clip.trim+offset,duration:clip.duration-offset}];}
/** @param {any} clip @param {string} id */
export function duplicateClip(clip,id){return {...structuredClone(clip),id,start:clip.start+clip.duration};}
/** @param {any} clip */
export function clipStyle(clip){const a={...defaults(),...clip.adjustments};const b=clip.overlay;const placement=b?`left:${b.x}%;top:${b.y}%;right:auto;bottom:auto;width:${b.width}%;height:${b.height}%;`:'';return `${placement}object-fit:${a.fit==='cover'?'cover':'contain'};object-position:${a.x}% ${a.y}%;transform:scale(${a.scale});transform-origin:${a.x}% ${a.y}%;filter:brightness(${a.brightness}) contrast(${a.contrast}) saturate(${a.saturation});opacity:${a.opacity}`;}
/** Drag a clip's left edge: later start, skip into the source, shorter clip. @param {any} clip @param {number} start @param {number} fps */
export function trimStart(clip,start,fps){const min=1/fps;const end=clip.start+clip.duration;const next=Math.max(Math.max(0,clip.start-(clip.trim??0)/(clip.speed||1)),Math.min(end-min,Math.round(start*fps)/fps));const delta=next-clip.start;return {...clip,start:next,trim:Math.max(0,(clip.trim??0)+delta*(clip.speed||1)),duration:end-next};}
/** Drag a clip's right edge; `limit` is the remaining source length when known. @param {any} clip @param {number} end @param {number} fps @param {number} [limit] */
export function trimEnd(clip,end,fps,limit){const min=1/fps;let duration=Math.max(min,Math.round((end-clip.start)*fps)/fps);if(limit&&limit>0)duration=Math.min(duration,limit);return {...clip,duration};}
/** Composition length that fits every clip, audio track and caption. @param {any} draft */
export function contentEnd(draft){return Math.max(0.1,...[...draft.clips,...draft.audio].map((/** @type {any} */ c)=>c.start+c.duration),...(draft.captions??[]).map((/** @type {any} */ c)=>c.end));}
