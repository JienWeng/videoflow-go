/** User-editable relationships. Generated provenance stays read-only. */
/** @param {import("@xyflow/svelte").Node|undefined} a @param {import("@xyflow/svelte").Node|undefined} b */
export function connectionRoute(a,b){
 if(!a||!b||a.id===b.id)return null;
 /** @param {string} x @param {string} y */
 const pair=(x,y)=>a.data.kind===x&&b.data.kind===y?[a,b]:a.data.kind===y&&b.data.kind===x?[b,a]:null;
 let p=pair('scene','character');if(p)return {path:`/scenes/${encodeURIComponent(p[0].id)}/cast/${encodeURIComponent(p[1].id)}`,label:'Cast member'};
 p=pair('shot','asset');if(p)return {path:`/shots/${encodeURIComponent(p[0].id)}/assets/${encodeURIComponent(p[1].id)}`,label:'Shot asset'};
 p=pair('scene','asset');if(p&&['prop','location'].includes(String(p[1].data.asset_type)))return {path:`/scenes/${encodeURIComponent(p[0].id)}/references/${encodeURIComponent(p[1].id)}`,scene:p[0].id,label:'Scene reference'};
 return null;
}
/** @param {import("@xyflow/svelte").Edge} edge @param {import("@xyflow/svelte").Node|undefined} a @param {import("@xyflow/svelte").Node|undefined} b */
export function editableConnection(edge,a,b){
 const route=connectionRoute(a,b);
 return route&&['casts','uses'].includes(String(edge.data?.rel))?route:null;
}
/** @type {Record<string,string>} */
export const relationshipLabels={casts:'Cast member',uses:'Uses asset',reference:'Character reference',shot:'Shot in scene',render:'Render job',output:'Rendered video',storyboard:'Generated storyboard',prop:'Generated prop',location:'Generated location'};
