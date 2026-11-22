// Populated Story canvas edit contract. Mock API only; no paid calls.
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
import {mkdir,writeFile} from 'node:fs/promises';
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:8091';
const out=process.env.E2E_OUTPUT||'/tmp/videoflow-canvas-connections';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});
const report={checks:[],errors:[]};
try {
for(const width of [1440,390]) {
 const context=await browser.newContext({viewport:{width,height:900}});const page=await context.newPage();page.setDefaultTimeout(30000);
 await context.addInitScript(()=>{localStorage.setItem('videoflow.onboarded','1');localStorage.setItem('videoflow.canvas.coach','1')});
 let scene={title:'Park',summary:'Red car in park',duration:5,aspect_ratio:'16:9'};
 const char={id:'c',name:'Alice',appearance:'Red coat',reference_images_json:[]};
 const graph=()=>({nodes:[{id:'s',type:'scene',label:scene.title,data:scene},{id:'t',type:'shot',label:'Closeup',data:{prompt:'Red car',duration:5,shot_order:0,scene_id:'s'}},{id:'c',type:'character',label:'Alice',data:char},{id:'a',type:'asset',label:'Car',data:{asset_type:'prop',file_path:'',metadata_json:{}}}],edges:[{source:'s',target:'t',label:'shot'},...(cast?[{source:'s',target:'c',label:'casts'}]:[]),...(linked?[{source:'s',target:'a',label:'uses'}]:[])]});
 page.on('pageerror',e=>report.errors.push(e.message));
 let patches=0;let chatRequests=[];let cast=false;let linked=false;
 await context.route('**/api/**',async r=>{
  const p=new URL(r.request().url()).pathname.replace('/api','');let body={};
  if(p==='/scenes/s/cast/c'){cast=r.request().method()==='POST';patches++;}else if(p==='/scenes/s/references/a'){linked=r.request().method()==='POST';if(!r.request().postDataJSON().expected_updated_at)throw Error('Missing concurrency revision');patches++;}else if(p==='/scenes/s')body={id:'s',...scene,updated_at:'2026-01-01T00:00:00Z'};else if(p==='/chat'){chatRequests.push(r.request().postDataJSON());body={intent:{action:'unknown',reply:'Review your selected item'},options:{},state:{}};}else if(p==='/characters/c')body=char;else if(p==='/assets/a')body={id:'a',updated_at:'2026-01-01',metadata_json:{locked_traits:{color:'red'}}};else if(p==='/graph')body=graph();else if(p==='/characters')body=[char];else if(p==='/assets'||p==='/scenes'||p==='/ops')body=[];
  else if(p==='/videos/preflight')body={ready:false,missing:[],effective_style:'Fixture',effective_routes:{image_provider:'fixture',image_model:'fixture',video_provider:'fixture',video_model:'fixture'},video_durations:[]};
  else if(p==='/projects/active')body={id:'p',name:'Canvas QA'};else if(p==='/projects')body=[];
  else if(p==='/director/sessions')body=[{id:'qa',selected:true,title:'Story planning',engine:'codex',status:'ready'}];else if(p==='/director/sessions/qa/turns')body=[];
  else if(p==='/director/status')body={all_text_local:true,engines:[]};
  else if(p==='/scenes/s'&&r.request().method()==='PATCH'){scene={...scene,...r.request().postDataJSON()};patches++;}
  else if(p==='/events')return r.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
  await r.fulfill({json:body});
 });
 await page.goto(base+'/?view=studio');
 const node=id=>page.locator(`.svelte-flow__node[data-id="${id}"]`);await node('s').waitFor();await page.waitForTimeout(800);
 await page.getByRole('button',{name:'Find canvas items',exact:true}).click();await page.getByRole('searchbox',{name:'Find canvas items',exact:true}).fill('Park');await page.getByRole('button',{name:'Park · Scene',exact:true}).click();
 if(width===390)await page.locator('[aria-label="Studio view"]').getByRole('button',{name:'Canvas',exact:true}).click();
 await page.getByRole('button',{name:'Manage selected item connections'}).click();const panel=page.getByRole('region',{name:'Item connections'});await panel.waitFor();
 await panel.getByLabel('Connect an existing item').selectOption('c');await panel.getByRole('button',{name:'Connect',exact:true}).focus();await page.keyboard.press('Enter');await panel.getByRole('button',{name:'Disconnect Alice'}).waitFor();if(!cast)throw Error('Cast did not persist');
 await panel.getByRole('button',{name:'Disconnect Alice'}).click();await panel.getByRole('button',{name:'Disconnect Alice'}).waitFor({state:'hidden'});if(cast)throw Error('Cast not detached');
 await panel.getByLabel('Connect an existing item').selectOption('a');await panel.getByRole('button',{name:'Connect',exact:true}).click();await panel.getByRole('button',{name:'Disconnect Car'}).waitFor();if(!linked)throw Error('Scene asset not persisted');
 if(await panel.getByRole('button',{name:'Disconnect Closeup'}).count())throw Error('Automatic link was editable');await panel.getByRole('button',{name:/Closeup/}).click();if(width===390)await page.locator('[aria-label="Studio view"]').getByRole('button',{name:'Canvas',exact:true}).click();await panel.getByRole('heading',{name:'Connections · Closeup',exact:true}).waitFor();
 report.checks.push(`${width}: keyboard cast attach, detach, revision-checked scene reference attach, automatic links protected, related-node navigation`);
 const {default:AxeBuilder}=await import('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs');const axe=await new AxeBuilder({page}).include('[aria-label="Item connections"]').withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();if(axe.violations.length)throw Error(JSON.stringify(axe.violations));
 await page.screenshot({path:`${out}/connections-${width}.png`});await context.close();
}
}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2));}
if(report.errors.length)throw Error(report.errors.join('\n'));console.log(JSON.stringify(report));
