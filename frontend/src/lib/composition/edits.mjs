/** Pure non-destructive timeline edits; source media is never rewritten. */
export const defaults=()=>({fit:'contain',scale:1,x:50,y:50,brightness:1,contrast:1,saturation:1,opacity:1});
/** @param {any} clip @param {number} time @param {string} id */
export function splitClip(clip,time,id){if(!Number.isFinite(time)||time<=clip.start+.001||time>=clip.start+clip.duration-.001)throw Error('Place the playhead inside the selected clip to split.');const offset=time-clip.start;return [{...structuredClone(clip),duration:offset},{...structuredClone(clip),id,start:time,trim:clip.trim+offset,duration:clip.duration-offset}];}
/** @param {any} clip @param {string} id */
export function duplicateClip(clip,id){return {...structuredClone(clip),id,start:clip.start+clip.duration};}
/** @param {any} clip */
export function clipStyle(clip){const a={...defaults(),...clip.adjustments};return `object-fit:${a.fit==='cover'?'cover':'contain'};object-position:${a.x}% ${a.y}%;transform:scale(${a.scale});transform-origin:${a.x}% ${a.y}%;filter:brightness(${a.brightness}) contrast(${a.contrast}) saturate(${a.saturation});opacity:${a.opacity}`;}
