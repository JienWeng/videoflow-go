/** Canonical editing destinations shared by every canvas node. */
/** @param {string} kind @param {string} id @param {Record<string,any>} data */
export function nodeTools(kind,id,data={}){
 const q=encodeURIComponent(id),scene=data.scene_id?encodeURIComponent(data.scene_id):'';
 if(kind==='character')return [{label:'Open character',href:`/characters/${q}`},{label:'Voice',href:`/voices?character_id=${q}`}];
 if(kind==='scene')return [{label:'Open scene in Story',href:`/?view=story&scene_id=${q}`},{label:'Edit scene video',href:`/?view=edit&scene_id=${q}`}];
 if(kind==='shot')return scene?[{label:'Open shot in Story',href:`/?view=story&scene_id=${scene}&shot_id=${q}`}]:[];
 if(kind==='asset')return [{label:'Open asset library',href:`/library?tab=media&asset_id=${q}`},...(/\.(png|jpe?g|webp|mp4|webm|mov|wav|mp3|m4a|ogg)$/i.test(data.file_path??'')?[{label:'Use in video editor',href:`/?view=edit&source=${q}`}]:[])];
 if(kind==='output')return [{label:'Edit video',href:`/?view=edit&source=${encodeURIComponent('output-'+id)}`},{label:'Classic caption editor',href:`/editor/${q}`}];
 if(kind==='final_video')return [{label:'Edit this video',href:data.script_id?`/?view=edit&script_id=${encodeURIComponent(data.script_id)}`:'/?view=edit'},{label:'Open Videos',href:'/render'}];
 if(kind==='render_job')return [{label:'Open Videos',href:'/render'},...(scene?[{label:'Open scene in Story',href:`/?view=story&scene_id=${scene}`}]:[])];
 return [];
}
