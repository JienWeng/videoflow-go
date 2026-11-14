// Mocked API scale/reflow/focus journeys plus axe WCAG checks. No provider calls.
import {mkdir,writeFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL(process.env.AXE_MODULE||'/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:8080',out=process.env.E2E_OUTPUT||'/tmp/videoflow-scale-accessibility';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
const report={checks:[],errors:[],accessibility:[],timings:[]};
try{for(const width of [1440,390]){
 const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'});await context.addInitScript(theme=>{localStorage.setItem('videoflow.onboarded','1');localStorage.setItem('videoflow.theme',theme)},process.env.E2E_THEME||'dark');
 const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>report.errors.push({url:page.url(),message:e.message}));let writes=0,populatedLists=false;
 const chars=Array.from({length:500},(_,i)=>({id:'c'+i,name:'Character '+i,appearance:'A distinctive red coat',reference_asset_ids_json:[]}));
 const history=Array.from({length:500},(_,i)=>({id:'turn'+i,kind:'chat',status:'succeeded',request_json:{user_message:'Question '+i},result_json:{reply:'Reply '+i+' '+('Story detail. '.repeat(20))}}));
 await context.route('**/api/**',async r=>{const p=new URL(r.request().url()).pathname.replace('/api','');let body={};if(r.request().method()!=='GET')writes++;
  if(p==='/events')return r.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
  if(p==='/graph')body={nodes:chars.map(c=>({id:c.id,type:'character',label:c.name,data:c})),edges:[]};
  else if(p==='/characters')body=chars;else if(p.startsWith('/characters/'))body=chars.find(c=>c.id===p.split('/').pop());
  else if(p==='/scenes')body=populatedLists?[{id:'s1',title:'First scene',summary:'A quiet opening',scene_order:0,character_ids_json:[],scene_json:{}},{id:'s2',title:'Second scene',summary:'A happy ending',scene_order:1,character_ids_json:[],scene_json:{}}]:[];
  else if(p==='/render-jobs')body=populatedLists?[{id:'j1',scene_id:'s1',status:'failed',created_at:'2026-11-14T00:00:00Z',request_json:{}},{id:'j2',scene_id:'s2',status:'failed',created_at:'2026-11-14T00:00:00Z',request_json:{}}]:[];
  else if(p==='/projects/active')body={id:'qa',name:'Scale QA'};else if(p==='/projects')body=[{id:'qa',name:'Scale QA',is_active:true}];
  else if(p==='/director/sessions')body=[{id:'qa',selected:true,title:'QA story',engine:'codex',status:'ready'}];else if(p==='/director/sessions/qa/turns')body=history;
  else if(p==='/director/status')body={all_text_local:true,engines:[]};else if(p==='/chat/capabilities')body={local:true,subagents:{supported:false,reason:'Fixture'}};
  else if(p==='/settings/app')body={all_text_local:true,render_negatives:[],default_aspect_ratio:'16:9',media_model_preferences:{}};
  else if(p==='/voice/state')body={profiles:[],scripts:[],tasks:[]};else if(p==='/assets/types')body={types:[]};else if(p==='/compositions/runtime')body={ready:false,message:'Fixture missing runtime'};
  else if(p==='/videos/preflight')body={missing:[],warnings:[],effective_routes:{},effective_style:'cinematic',video_durations:[5]};
  else if(p==='/health')body={status:'ok'};else if(p==='/style')body={};else body=[];
  await r.fulfill({json:body});
 });
 const check=async(name,fn)=>{await fn();report.checks.push({name:width+': '+name,status:'pass'});console.log('PASS',width,name)};
 if(!process.env.E2E_A11Y_ONLY){
 await check('500-node canvas loads, finder selects final item and inspector opens',async()=>{const t=Date.now();await page.goto(base+'/?view=studio');await page.locator('.svelte-flow__node').first().waitFor();report.timings.push({width,kind:'500-node-load-ms',value:Date.now()-t});if(width===390)await page.getByRole('button',{name:'Canvas',exact:true}).click();await page.getByRole('button',{name:'Find canvas items',exact:true}).click();await page.getByRole('searchbox',{name:'Find canvas items',exact:true}).fill('Character 499');await page.getByRole('button',{name:/Character 499/}).click();await page.locator('#np-character-name').waitFor();assert.equal(await page.locator('#np-character-name').inputValue(),'Character 499');});
 await check('500-turn chat renders latest history, retains reading position and accepts draft',async()=>{await page.getByRole('tab',{name:'Chat',exact:true}).click();await page.getByText('Question 499',{exact:true}).waitFor();const log=page.getByRole('log',{name:'Story conversation'});await log.evaluate(n=>n.scrollTop=0);await page.getByRole('button',{name:'Jump to latest message'}).waitFor();const input=page.locator('textarea').last();await input.fill('Keep this unsent scale draft');assert.equal(await input.inputValue(),'Keep this unsent scale draft');await page.getByRole('button',{name:'Jump to latest message'}).click();await page.waitForFunction(()=>{const n=document.querySelector('[data-slot="message-scroller-viewport"]');return n&&n.scrollHeight-n.scrollTop-n.clientHeight<60});assert.equal(writes,0);});
 }
 chars.splice(2);history.splice(5); // Scan representative content after the separate scale assertions.
 for(const route of (process.env.E2E_A11Y_ROUTES?.split(',')||['/?view=studio','/create','/render','/projects','/characters','/assets','/scenes','/settings','/director','/voices','/compositions'])){
  await page.goto(base+route);await page.waitForFunction(()=>document.body.innerText.length>40);await page.waitForLoadState('networkidle');await page.waitForTimeout(200);
  const results=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();console.log('SCAN',width,route,results.violations.map(v=>v.id).join(','));report.accessibility.push({width,route,violations:results.violations.map(v=>({id:v.id,impact:v.impact,description:v.description,nodes:v.nodes.map(n=>({target:n.target,html:n.html,summary:n.failureSummary}))})),incomplete:results.incomplete.map(v=>({id:v.id,nodes:v.nodes.length}))});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false,route+' overflow');report.checks.push({name:width+': '+route+' responsive reflow',status:'pass'});
 }
 await check('Scene and render button groups expose selection and preserve arrow navigation',async()=>{populatedLists=true;for(const [route,name] of [['/scenes','Scenes'],['/render','Scenes with renders']]){await page.goto(base+route);const group=page.getByRole('group',{name,exact:true});await group.getByRole('button',{pressed:true}).waitFor();await group.focus();await page.keyboard.press('ArrowDown');assert.notEqual(await group.evaluate(n=>getComputedStyle(n).outlineStyle),'none','Focused group has no visible outline');assert.equal(await group.getByRole('button',{pressed:true}).count(),1);assert.match(await group.getByRole('button',{pressed:true}).innerText(),/Second scene/);const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();report.accessibility.push({width,route:route+' populated',violations:result.violations,incomplete:result.incomplete.map(v=>({id:v.id,nodes:v.nodes.length}))});}populatedLists=false;});
 await check('Creation dialog focus trap, Escape and trigger focus restoration',async()=>{await page.goto(base+'/characters');const trigger=page.getByRole('button',{name:'Create character',exact:true}).first();await trigger.click();const dialog=page.getByRole('dialog');await dialog.waitFor();for(let i=0;i<10;i++){await page.keyboard.press('Tab');assert(await dialog.evaluate(n=>n.contains(document.activeElement)),'Focus escaped dialog');}await page.keyboard.press('Escape');await dialog.waitFor({state:'hidden'});assert(await trigger.evaluate(n=>n===document.activeElement));});
 await context.close();
}}catch(e){report.failure=e.stack;process.exitCode=1;console.error(e)}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2));}if(report.errors.length||report.accessibility.some(a=>a.violations.length))process.exitCode=1;
