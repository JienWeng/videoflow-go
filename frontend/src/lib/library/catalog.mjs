/** @param {{file_path?:string|null,type?:string|null}} asset */
export function mediaKind(asset) {
 const path=asset.file_path??'';
 if(/\.(mp4|webm|mov|m4v|avi)$/i.test(path)||asset.type==='video')return 'video';
 if(/\.(mp3|m4a|aac|wav|ogg|flac|opus)$/i.test(path)||asset.type==='audio')return 'audio';
 if(/\.(png|jpe?g|webp|gif|avif|svg)$/i.test(path)||asset.type==='image')return 'image';
 return 'other';
}
/** @param {string|null|undefined} value */
export function typeLabel(value) {const text=(value||'other').replaceAll('_',' ');return text.charAt(0).toUpperCase()+text.slice(1);}
/** @param {{name?:string,description?:string,appearance?:string,personality?:string}} character @param {string} query */
export function characterMatches(character,query) {return [character.name,character.description,character.appearance,character.personality].join(' ').toLocaleLowerCase().includes(query.trim().toLocaleLowerCase());}
