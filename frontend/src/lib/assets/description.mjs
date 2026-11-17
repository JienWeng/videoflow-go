/** @typedef {{name:string,id:string,description:string,appearance:string,personality:string,visual_rules:string,voice_rules:string,sample_dialogue:string}} DescriptionCharacter */
/** @typedef {{summary:string,style:string,language:string,mode:string,direction:string,characters:DescriptionCharacter[]}} AssetDescription */
const labels={style:'Visual style',language:'Spoken language',mode:'Mode',direction:'Direction'};
const characterLabels={description:'Description',appearance:'Appearance',personality:'Personality',visual_rules:'Visual rules',voice_rules:'Voice rules',sample_dialogue:'Sample dialogue'};
/** @param {string} value */
const clean=value=>{const text=value.trim();if(['[]','[].','.'].includes(text))return '';return text.endsWith('..')&&!text.endsWith('...')?text.slice(0,-1):text;};
/** Parse both existing generated prompts and the canonical editable description.
 * Unrecognised prose is retained; no HTML/Markdown is evaluated.
 * @param {string|null|undefined} source @returns {AssetDescription}
 */
export function parseDescription(source){
 const result={summary:'',style:'',language:'',mode:'',direction:'',characters:/** @type {DescriptionCharacter[]} */([])};
 /** @type {{summary:string[],style:string[],language:string[],mode:string[],direction:string[]}} */
 const sections={summary:[],style:[],language:[],mode:[],direction:[]};
 let current=/** @type {keyof typeof sections} */('summary');
 let character=/** @type {{name:string,id:string,lines:string[]}|null} */(null);
 function finish(){if(!character)return;const body=character.lines.join('\n');const fields={description:'',appearance:'',personality:'',visual_rules:'',voice_rules:'',sample_dialogue:''};let key=/** @type {keyof typeof fields} */('description'),offset=0;
  for(const match of body.matchAll(/\b(Description|Appearance|Personality|Visual rules|Voice rules|Sample dialogue):\s*/g)){fields[key]+=(fields[key]?'\n':'')+body.slice(offset,match.index);key=/** @type {keyof typeof fields} */(match[1].toLowerCase().replaceAll(' ','_'));offset=match.index+match[0].length;}
  fields[key]+=(fields[key]?'\n':'')+body.slice(offset);for(const k of Object.keys(fields))fields[/** @type {keyof typeof fields} */(k)]=clean(fields[/** @type {keyof typeof fields} */(k)]);
  result.characters.push({name:character.name,id:character.id,...fields});character=null;
 }
 for(const line of (source??'').replaceAll('\r\n','\n').split('\n')){
  const actor=line.match(/^Character\s+(.+?)(?:\s+\(([^)]+)\))?:\s*(.*)$/);
  if(actor){finish();character={name:actor[1].trim(),id:actor[2]??'',lines:[actor[3]]};continue}
  const field=line.match(/^(Visual style|Spoken language|Mode|Direction):\s*(.*)$/i);
  if(field){finish();current=/** @type {keyof typeof sections} */(Object.entries(labels).find(([,label])=>label.toLowerCase()===field[1].toLowerCase())?.[0]??'summary');sections[current].push(field[2]);continue}
  if(character)character.lines.push(line);else sections[current].push(line);
 }
 finish();for(const key of Object.keys(sections))result[/** @type {keyof typeof sections} */(key)]=sections[/** @type {keyof typeof sections} */(key)].join('\n').trim();return result;
}
/** Canonical labelled text preserves compatibility with generation and existing asset APIs.
 * @param {AssetDescription} value @returns {string}
 */
export function formatDescription(value){const parts=[value.summary.trim()];for(const [key,label] of Object.entries(labels)){const text=value[/** @type {keyof typeof labels} */(key)].trim();if(text)parts.push(`${label}: ${text}`)}for(const c of value.characters){const lines=[`Character ${c.name}${c.id?` (${c.id})`:''}:`];for(const [key,label] of Object.entries(characterLabels)){const text=c[/** @type {keyof typeof characterLabels} */(key)].trim();if(text)lines.push(`${label}: ${text}`)}parts.push(lines.join('\n'))}return parts.filter(Boolean).join('\n\n')}
