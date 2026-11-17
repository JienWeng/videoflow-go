/** @typedef {{idea:string,instruction:string,style_preset_id:string,style_fingerprint:string,tone:string,audience:string,platform:string,aspect_ratio:string,resolution:string,language:string,conversation_mode:string,target_duration:number,scene_count:number}} CreativeBrief */
/** @param {any} [value] @param {string} [idea] @returns {CreativeBrief} */
export function normalizeBrief(value={},idea=''){
 /** @param {string} key */ const text=key=>typeof value?.[key]==='string'?value[key]:'';
 return {idea:text('idea')||idea,instruction:text('instruction'),style_preset_id:text('style_preset_id'),style_fingerprint:text('style_fingerprint'),tone:text('tone'),audience:text('audience'),platform:text('platform'),aspect_ratio:text('aspect_ratio'),resolution:text('resolution'),language:text('language'),conversation_mode:text('conversation_mode'),target_duration:Number.isInteger(value?.target_duration)?value.target_duration:0,scene_count:Number.isInteger(value?.scene_count)?value.scene_count:0};
}
/** @param {CreativeBrief} value @returns {string[]} */
export function missingBriefFields(value){
 const missing=[];for(const [key,label]of Object.entries({idea:'Story idea',style_preset_id:'Visual style',style_fingerprint:'Current style selection',tone:'Tone',audience:'Audience',platform:'Platform',aspect_ratio:'Format',resolution:'Size',language:'Language',conversation_mode:'Dialogue mode'}))if(!String(/** @type {any} */(value)[key]??'').trim())missing.push(label);
 if(!['9:16','16:9','1:1','3:4','4:3','21:9'].includes(value.aspect_ratio))missing.push('Supported format');if(!['auto','480p','720p'].includes(value.resolution))missing.push('Supported video size');if(!['dialogue','narration','silent'].includes(value.conversation_mode))missing.push('Speech choice');
 if(!Number.isInteger(value.scene_count)||value.scene_count<1||value.scene_count>20)missing.push('Scene count (1–20)');
 if(!Number.isInteger(value.target_duration)||value.target_duration<Math.max(1,value.scene_count)||value.target_duration>600)missing.push('Total duration (at least one second per scene, up to 600 seconds)');return missing;
}
/** @param {CreativeBrief} value @param {boolean} confirmed */
export function confirmedBriefRequest(value,confirmed){const missing=missingBriefFields(value);if(!confirmed||missing.length)throw Error(missing.length?'Complete the creative brief: '+missing.join(', '):'Confirm the creative brief before creating');return {...value,brief_version:1,brief_confirmed:true};}
