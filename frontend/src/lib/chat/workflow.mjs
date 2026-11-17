/** Suggest prerequisites from current saved records, never claim a generation already ran.
 * @param {{nodes?:any[],edges?:any[]}|null} graph
 * @returns {{label:string;send?:string;href?:string;focusId:string}[]}
 */
export function workflowNext(graph){
 const nodes=graph?.nodes??[],edges=graph?.edges??[],hasScenes=nodes.some(n=>n.type==='scene');const byId=new Map(nodes.map(n=>[n.id,n]));
 const cast=new Set(edges.filter(e=>e.label==='casts').map(e=>e.target));
 const result=[];
 for(const c of nodes.filter(n=>n.type==='character'&&(!hasScenes||cast.has(n.id)))){
 const refs=Array.isArray(c.data?.reference_asset_ids_json)?c.data.reference_asset_ids_json:[];
 if(!refs.length)result.push({label:`Create ${c.label}’s reference image`,send:`Generate a full-body character reference image for ${c.label}.`,focusId:c.id});
 else if(refs.some((/** @type {string} */ id)=>!byId.has(id)||['rejected'].includes(byId.get(id)?.data?.consistency_review)))result.push({label:`Replace ${c.label}’s excluded images`,href:`/characters/${encodeURIComponent(c.id)}#np-character-references`,focusId:c.id});
 if(result.length===2)break;
 }return result;
}
/** Only current character bindings count; rejected superseded candidates never block continuation.
 * @param {string[]} castIds @param {string[]} assetIds @param {any[]} characters @param {any[]} assets
 */
export function storyReferencesApproved(castIds,assetIds,characters,assets){const cast=castIds.map(id=>characters.find(c=>c.id===id));if(!cast.every(c=>c&&c.reference_asset_ids_json?.length))return false;const ids=[...new Set([...assetIds,...cast.flatMap(c=>c.reference_asset_ids_json)])];return ids.every(id=>{const asset=assets.find(a=>a.id===id);return asset&&asset.metadata_json?.consistency_review!=='rejected'})}
/** @param {{nodes?:any[],edges?:any[]}|null} graph @param {string} sceneId */
export function hasPendingSceneReferences(graph,sceneId){const nodes=new Map((graph?.nodes??[]).map(n=>[n.id,n]));return (graph?.edges??[]).some(e=>{const id=e.source===sceneId?e.target:e.target===sceneId?e.source:null;const node=nodes.get(id);return node?.type==='asset'&&['rejected'].includes(node.data?.consistency_review)})}
