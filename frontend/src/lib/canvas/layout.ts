import type { Node, Edge } from '@xyflow/svelte';
const W=180,H=60;
const IMG_RE=/\.(png|jpe?g|webp|gif)$/i;
export function nodeSize(n: Node): { w: number; h: number } {
  const d = (n.data ?? {}) as Record<string, unknown>;
  const kind = String(d.kind ?? '');
  let h = H;
  const hasPreview =
    (kind === 'asset' && IMG_RE.test(String(d.file_path ?? ''))) ||
    (kind === 'output' && !!d.thumbnail_path) || (kind==='character'&&!!d.reference_preview_path);
  if (hasPreview) h += 70;
  if (kind === 'render_job') h += 26;
  if(kind==='character')h+=58;
  if(kind==='final_video')h+=40;
  if(kind==='scene'&&d.episode_number)h+=18;
  return { w: kind === 'scene' ? 190 : W, h };
}

type Pos={x:number;y:number};
const GAP=36,COLUMN=290,COLS=6;
const kind=(n:Node)=>String(n.data?.kind??'');
function compare(a:Node,b:Node){
 const field=kind(a)==='scene'?'scene_order':kind(a)==='shot'?'shot_order':null;
 const av=field?Number(a.data?.[field]):NaN,bv=field?Number(b.data?.[field]):NaN;
 if(Number.isFinite(av)&&Number.isFinite(bv)&&av!==bv)return av-bv;
 return String(a.data?.label??a.id).localeCompare(String(b.data?.label??b.id), 'en',{numeric:true})||a.id.localeCompare(b.id);
}
/** Semantic stage columns keep shots in order, and shared cast out of shot ranks. */
export function layout(nodes:Node[],edges:Edge[]):Node[]{
 const sorted=nodes.slice().sort((a,b)=>kind(a).localeCompare(kind(b))||compare(a,b));
 const byId=new Map(nodes.map(n=>[n.id,n]));const positions=new Map<string,Pos>();
 const adjacent=new Map(nodes.map(n=>[n.id,[] as string[]]));
 for(const e of edges)if(byId.has(e.source)&&byId.has(e.target)){adjacent.get(e.source)!.push(e.target);adjacent.get(e.target)!.push(e.source)}
 const characters=sorted.filter(n=>kind(n)==='character');const references=new Set<string>();
 const refs=new Map<string,string[]>();
 for(const c of characters){const ids=[...(Array.isArray(c.data?.reference_asset_ids_json)?c.data.reference_asset_ids_json as string[]:[]),...adjacent.get(c.id)!].filter(id=>kind(byId.get(id)??c)==='asset');const unique=[...new Set(ids)].filter(id=>byId.has(id)).sort((a,b)=>compare(byId.get(a)!,byId.get(b)!));refs.set(c.id,unique);unique.forEach(id=>references.add(id));}
 const scenes=sorted.filter(n=>kind(n)==='scene');const owner=new Map<string,string>();
 // Explicit scene ownership wins. Shared hubs never move another scene's shots.
 for(const n of sorted){const id=String(n.data?.scene_id??(n.data?.metadata_json as any)?.scene_id??'');if(scenes.some(s=>s.id===id)&&kind(n)!=='character'&&!references.has(n.id))owner.set(n.id,id)}
 for(const scene of scenes)owner.set(scene.id,scene.id);
 let frontier=scenes.flatMap(scene=>sorted.filter(n=>owner.get(n.id)===scene.id).map(n=>n.id));
 while(frontier.length){const next:string[]=[];for(const id of frontier)for(const other of adjacent.get(id)??[]){if(owner.has(other)||kind(byId.get(other)!)==='character'||references.has(other))continue;owner.set(other,owner.get(id)!);next.push(other)}frontier=next}
 let top=0;
 for(const scene of scenes){let bottom=top;const group=sorted.filter(n=>owner.get(n.id)===scene.id);const columns=[['scene'],['shot'],['asset'],['render_job'],['output'],['final_video']];
  columns.forEach((kinds,column)=>{let y=top;for(const n of group.filter(n=>kinds.includes(kind(n))).sort(compare)){positions.set(n.id,{x:column*COLUMN,y});y+=nodeSize(n).h+GAP;bottom=Math.max(bottom,y)}});top=bottom+80;
 }
 // Character/reference pairs form a separate band, not branches between shots.
 for(const c of characters){const left=scenes.length?COLUMN:0;let height=nodeSize(c).h;positions.set(c.id,{x:left,y:top});let col=1;for(const id of refs.get(c.id)??[]){if(positions.has(id))continue;const n=byId.get(id)!;positions.set(id,{x:left+col++*COLUMN,y:top});height=Math.max(height,nodeSize(n).h)}top+=height+GAP}
 if(characters.length)top+=80;
 const remaining=sorted.filter(n=>!positions.has(n.id));
 for(let start=0;start<remaining.length;start+=COLS){const row=remaining.slice(start,start+COLS);row.forEach((n,i)=>positions.set(n.id,{x:i*COLUMN,y:top}));top+=Math.max(...row.map(n=>nodeSize(n).h))+GAP}
 return nodes.map(n=>({...n,position:positions.get(n.id)!}));
}
/** Preserve manual positions and fit new cards around actual occupied bounds. */
export function layoutFresh(nodes:Node[],edges:Edge[],saved:Record<string,Pos>):Node[]{
 const valid=(id:string)=>saved[id]&&Number.isFinite(saved[id].x)&&Number.isFinite(saved[id].y);
 const occupied=nodes.filter(n=>valid(n.id)).map(n=>({...saved[n.id],...nodeSize(n)}));
 return layout(nodes,edges).map(n=>{if(valid(n.id))return {...n,position:{...saved[n.id]}};const size=nodeSize(n),p={...n.position};let collisions;
  do{collisions=occupied.filter(r=>p.x<r.x+r.w+GAP&&p.x+size.w+GAP>r.x&&p.y<r.y+r.h+GAP&&p.y+size.h+GAP>r.y);if(collisions.length)p.y=Math.max(...collisions.map(r=>r.y+r.h+GAP))}while(collisions.length);
  occupied.push({...p,...size});return {...n,position:p};
 });
}
