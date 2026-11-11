// Populated Story canvas edit contract. Mock API only; no paid calls.
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
import {mkdir,writeFile} from 'node:fs/promises';
const out='/tmp/videoflow-canvas-audit';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
const report={checks:[],errors:[]};
try {
for(const width of [1440,390]) {
 const context=await browser.newContext({viewport:{width,height:900}});const page=await context.newPage();page.setDefaultTimeout(7000);
 await context.addInitScript(()=>{localStorage.setItem('videoflow.onboarded','1');localStorage.setItem('videoflow.canvas.coach','1')});
 let scene={title:'Park',summary:'Red car in park',duration:5,aspect_ratio:'16:9'};
 const char={id:'c',name:'Alice',appearance:'Red coat',reference_images_json:[]};
 const graph=()=>({nodes:[{id:'s',type:'scene',label:scene.title,data:scene},{id:'t',type:'shot',label:'Closeup',data:{prompt:'Red car',duration:5,shot_order:0,scene_id:'s'}},{id:'c',type:'character',label:'Alice',data:char},{id:'a',type:'asset',label:'Car',data:{asset_type:'image',file_path:'',metadata_json:{}}}],edges:[{source:'s',target:'t',label:'shot'},{source:'c',target:'s',label:'casts'}]});
 page.on('pageerror',e=>report.errors.push(e.message));
 let patches=0;
 await context.route('**/api/**',async r=>{
  const p=new URL(r.request().url()).pathname.replace('/api','');let body={};
  if(p==='/graph')body=graph();else if(p==='/characters')body=[char];else if(p==='/assets'||p==='/scenes'||p==='/ops'||p==='/director/sessions')body=[];
  else if(p==='/projects/active')body={id:'p',name:'Canvas QA'};else if(p==='/projects')body=[];
  else if(p==='/director/status')body={all_text_local:true,engines:[]};
  else if(p==='/scenes/s'&&r.request().method()==='PATCH'){scene={...scene,...r.request().postDataJSON()};patches++;}
  else if(p==='/events')return r.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
  await r.fulfill({json:body});
 });
 await page.goto('http://127.0.0.1:5194');
 const node=id=>page.locator(`.svelte-flow__node[data-id="${id}"]`);
 await node('s').waitFor();await page.waitForTimeout(800);
 // On mobile Studio deliberately offers a focused canvas view.
 const canvasToggle=page.getByRole('button',{name:'Canvas',exact:true});if(await canvasToggle.isVisible())await canvasToggle.click();

 await node('s').click({force:true});await page.locator('#np-title').fill('Edited park');await page.getByTestId('node-panel').getByRole('button',{name:'Save',exact:true}).click();
 if(patches!==1)throw Error('Scene edit not saved');report.checks.push(`${width}: scene edit/save`);
 await page.getByTestId('node-panel').getByRole('button',{name:'Close',exact:true}).click();
 await node('t').click({force:true});await page.locator('#np-prompt').waitFor();report.checks.push(`${width}: shot editor opens`);
 await page.getByTestId('node-panel').getByRole('button',{name:'Close',exact:true}).click();
 await page.getByRole('button',{name:'fit view',exact:false}).click();await page.waitForTimeout(400);await node('c').click();await page.getByRole('link',{name:'Edit character',exact:true}).click();await page.locator('#edit-appear-c').waitFor();if(await page.locator('#edit-appear-c').inputValue()!=='Red coat')throw Error('Character deep link lost data');report.checks.push(`${width}: character deep link opens correct editor`);
 await page.goto('http://127.0.0.1:5194');if(await canvasToggle.isVisible())await canvasToggle.click();await node('a').click({force:true});
 if(await page.getByRole('link',{name:'Edit asset identity & review'}).getAttribute('href')!=='/assets?asset_id=a')throw Error('Wrong asset editor');report.checks.push(`${width}: asset identity editor link`);
 await page.getByTestId('node-panel').getByRole('button',{name:'Close',exact:true}).click();
 await node('s').click({button:'right',force:true});const menu=page.getByTestId('canvas-context-menu');await menu.waitFor();const b=await menu.boundingBox();if(b.x<0||b.y<0||b.y+b.height>900)throw Error('Context menu offscreen');await page.keyboard.press('Escape');report.checks.push(`${width}: context menu visible and Escape closes`);
 const before=await node('s').boundingBox();await page.mouse.move(before.x+before.width/2,before.y+before.height/2);await page.mouse.down();await page.mouse.move(before.x+before.width/2+35,before.y+before.height/2+25,{steps:8});await page.mouse.up();const positions=await page.evaluate(()=>JSON.parse(localStorage.getItem('videoflow.canvas.positions')||'{}'));if(!positions.s)throw Error('Drag position not saved');report.checks.push(`${width}: node drag persists position`);
 await page.screenshot({path:`${out}/canvas-${width}.png`});await context.close();
}
}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2));}
console.log(report);if(report.errors.length)process.exitCode=1;
