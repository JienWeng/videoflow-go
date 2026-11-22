export const layouts=[
 {name:'Clean story',width:1920,height:1080,title:'Your story',position:'center'},
 {name:'Vertical short',width:1080,height:1920,title:'A small story',position:'top'},
 {name:'Product spotlight',width:1080,height:1080,title:'Meet the difference',position:'bottom'},
 {name:'Lesson opener',width:1920,height:1080,title:'What you will learn',position:'center'}
];
/** @param {any} value */
export function readLayout(value){if(value?.format!=='videoflow-layout'||value.version!==1)throw Error('Use a VideoFlow layout JSON file (version 1).');const p=value.layout;if(!p||typeof p.name!=='string'||!p.name.trim()||p.name.length>80||!Number.isInteger(p.width)||!Number.isInteger(p.height)||p.width<16||p.width>3840||p.height<16||p.height>3840||typeof p.title!=='string'||p.title.length>500||!['top','center','bottom'].includes(p.position))throw Error('Layout contains invalid dimensions or title settings');return {name:p.name,width:p.width,height:p.height,title:p.title,position:p.position}}
/** @param {any} draft @param {any} layout @param {any} brand */
export function useLayout(draft,layout,brand){const title={id:'title-'+crypto.randomUUID(),text:layout.title||'Your title',start:0,end:Math.min(3,draft.duration),size:48,color:brand?.text||'#FFFFFF',background:brand?.primary||'#111827',font:brand?.font||'sans-serif',position:layout.position};return {...draft,width:layout.width,height:layout.height,texts:[...(draft.texts??[]),title]}}
