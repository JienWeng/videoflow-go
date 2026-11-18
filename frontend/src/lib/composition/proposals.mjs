/** Summarize editable differences without mutating either manifest. */
/** @param {any} before @param {any} after */
export function editChanges(before,after){
 const changes=[];
 for(const field of ['width','height','fps','duration','caption_style'])if(before[field]!==after[field])changes.push(`${field}: ${before[field]} → ${after[field]}`);
 for(const track of ['clips','audio','captions','texts']){
  const old=new Map((before[track]??[]).map((/** @type {any} */ item)=>[item.id,item]));
  for(const item of after[track]??[]){const original=old.get(item.id);if(!original)changes.push(`Add ${track}: ${item.id}`);else {const fields=['asset_id','start','end','trim','duration','volume','muted','fade_in','fade_out','text','size','color','background','font','position','adjustments','overlay'];const changed=fields.filter(key=>JSON.stringify(original[key])!==JSON.stringify(item[key]));if(changed.length)changes.push(`${track} ${item.id}: ${changed.map(key=>`${key} ${String(original[key]??'default').slice(0,60)} → ${String(item[key]??'default').slice(0,60)}`).join('; ')}`)}old.delete(item.id)}
  for(const id of old.keys())changes.push(`Remove ${track}: ${id}`);
  if((before[track]??[]).length===(after[track]??[]).length&&JSON.stringify((before[track]??[]).map((/** @type {any} */ item)=>item.id))!==JSON.stringify((after[track]??[]).map((/** @type {any} */ item)=>item.id)))changes.push(`${track}: layer order changed`);
 }
 return changes;
}
/** @param {any} current @param {any} proposal */
export function applyEditProposal(current,proposal){if(JSON.stringify(current)!==proposal.base)throw Error('The edit changed after this proposal. Ask for a new proposal.');return JSON.parse(JSON.stringify(proposal.manifest))}
/** Move within bounds and snap to frame/neighbor edges. Linked cues must be detached first. */
/** @param {any} draft @param {string} track @param {string} id @param {number} start @param {boolean} [snapEdges] */
export function moveClip(draft,track,id,start,snapEdges=true){
 const clip=draft[track].find((/** @type {any} */ c)=>c.id===id);if(!clip)throw Error('Choose a clip');
 if(draft.caption_bindings?.some((/** @type {any} */ b)=>b.clip_id===id))throw Error('Detach linked captions before moving this clip.');
 const edges=[0,...draft[track].filter((/** @type {any} */ c)=>c.id!==id).flatMap((/** @type {any} */ c)=>[c.start,c.start+c.duration])];
 let next=Math.round(start*draft.fps)/draft.fps;
 const snap=edges.find(edge=>Math.abs(edge-next)<=.15);if(snapEdges&&snap!==undefined)next=snap;
 next=Math.max(0,Math.min(draft.duration-clip.duration,next));
 return {...draft,[track]:draft[track].map((/** @type {any} */ c)=>c.id===id?{...c,start:next}:c)};
}
