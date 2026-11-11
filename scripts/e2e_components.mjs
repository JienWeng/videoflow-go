/** Deterministic UI contract. Requires external Playwright; never submits real inference.
 * PLAYWRIGHT_MODULE=/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs node scripts/e2e_components.mjs
 */
import {mkdir, writeFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE || '/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const base=process.env.E2E_BASE_URL || 'http://127.0.0.1:5194';
const out=process.env.E2E_OUTPUT || '/tmp/videoflow-component-e2e';
await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH || '/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
const report={base,started:new Date().toISOString(),checks:[],inventories:[],errors:[],unknownEndpoints:[]};
const routes=['/','/create','/projects','/scenes','/characters','/assets','/render','/settings','/director','/voices','/compositions','/editor/missing-output'];
const assert=(v,m)=>{if(!v)throw Error(m)};
async function check(name,fn){try{await fn();report.checks.push({name,status:'pass'});console.error('PASS',name)}catch(e){report.checks.push({name,status:'fail',error:e.message});console.error('FAIL',name,e.message)}}
async function inventory(page,route,mode,state){
 const controls=await page.locator('button,a,input,select,textarea,summary,[role="tab"],[role="checkbox"]').evaluateAll(nodes=>nodes.map(n=>({tag:n.tagName,role:n.getAttribute('role'),name:n.getAttribute('aria-label')||n.getAttribute('title')||n.innerText?.trim()||n.getAttribute('placeholder')||n.id,type:n.getAttribute('type'),disabled:n.disabled||false,visible:!!(n.offsetWidth||n.offsetHeight||n.getClientRects().length),href:n.getAttribute('href')})));
 report.inventories.push({route,mode,state,controls});
}
try{
for(const mode of ['desktop','mobile']){
 const context=await browser.newContext({viewport:mode==='desktop'?{width:1440,height:1000}:{width:390,height:844}});
 await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));
 let projects=[{id:'active',name:'Fixture active',is_active:true,created_at:'2026-01-01',counts:{}},{id:'spare',name:'Fixture spare',is_active:false,created_at:'2026-01-01',counts:{}}];
 let missing=false;
 await context.route('**/*',async route=>{
  const req=route.request(),u=new URL(req.url());
  if(!u.pathname.startsWith('/api/') && u.port!=='8000'){
   if(u.origin!==new URL(base).origin)return route.abort();
   return route.continue();
  }
  const p=u.pathname.replace(/^\/api/,'');
  if(missing)return route.fulfill({status:503,json:{error:'Fixture backend unavailable'}});
  let body;
  if(p==='/projects/active')body=projects[0];
  else if(p==='/projects')body=projects;
  else if(p.startsWith('/projects/')&&req.method()==='PATCH'){const value=req.postDataJSON();projects=projects.map(x=>x.id===p.split('/')[2]?{...x,...value}:x);body={};}
  else if(p.startsWith('/projects/')&&req.method()==='DELETE'){projects=projects.filter(x=>x.id!==p.split('/')[2]);body={};}
  else if(['/characters','/scenes','/assets','/scripts','/render-jobs','/ops','/settings/agents','/settings/providers','/settings/connection-presets'].includes(p))body=[];
  else if(p==='/settings/app')body={default_video_provider:'openrouter',default_image_provider:'openrouter',render_negatives:[],default_negatives:[],default_aspect_ratio:'16:9',engines:{}};
  else if(p==='/assets/types')body={types:[]};
  else if(p==='/director/status')body={all_text_local:true,engines:[],host_requirement:'Fixture backend host'};
  else if(p==='/director/sessions')body=[];
  else if(p==='/voice/state')body={project_id:'active',profiles:[],scripts:[],tasks:[]};
  else if(p==='/compositions/sources')body=[];
  else if(p==='/compositions/runtime')body={ready:false,message:'Fixture optional runtime missing'};
  else if(p==='/style')body={};
  else if(p==='/graph')body={nodes:[],edges:[]};
  else if(p==='/events')return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
  else if(p==='/generation-requests')body=[];
  else if(p==='/caption-config')body={};
  else if(p==='/videos/preflight')body={effective_style:'cinematic',effective_routes:{},missing:['Fixture has no inference provider'],warnings:[]};
  else if(p.includes('missing-output'))return route.fulfill({status:404,json:{error:'Output not found'}});
  else {report.unknownEndpoints.push({path:p,method:req.method()});return route.fulfill({status:404,json:{error:'Unmocked fixture endpoint: '+p}});}
  return route.fulfill({json:body});
 });
 const page=await context.newPage();page.setDefaultTimeout(5000);page.on('pageerror',e=>report.errors.push({mode,error:e.message,url:page.url()}));
 for(const route of routes)await check(`${mode} ${route}: load, inventory, palette`,async()=>{
  await page.goto(base+route,{waitUntil:'domcontentloaded'});await page.waitForFunction(()=>document.body.innerText.length>20);await page.waitForTimeout(250);assert(await page.locator('body').innerText(),'Empty body');
  await inventory(page,route,mode,'empty');
  await page.keyboard.press('Control+k');await page.getByRole('dialog').waitFor({timeout:3000});
  await inventory(page,route,mode,'command-palette');await page.keyboard.press('Escape');
  await page.getByRole('dialog').waitFor({state:'hidden'});
  await page.screenshot({path:`${out}/${mode}-${route.replaceAll('/','_')||'studio'}.png`,fullPage:true});
 });
 await check(`${mode} projects: required name, rename cancel/save, delete cancel/confirm`,async()=>{
  await page.goto(base+'/projects',{waitUntil:'domcontentloaded'});await page.getByTitle('Rename project').nth(1).waitFor();
  assert(await page.getByRole('button',{name:'Create project',exact:true}).isDisabled(),'Empty project allowed');
  await page.getByTitle('Rename project').nth(1).click();await page.getByTitle('Cancel',{exact:true}).click();
  await page.getByTitle('Rename project').nth(1).click();await page.locator('form input').last().fill('Renamed fixture');await page.getByTitle('Save name').click();await page.getByText('Renamed fixture',{exact:true}).waitFor();
  await page.getByRole('button',{name:'Delete',exact:true}).click();await inventory(page,'/projects',mode,'delete-confirmation');
  await page.getByRole('button',{name:'Cancel',exact:true}).click();assert(projects.length===2,'Cancel mutated project');
  await page.getByRole('button',{name:'Delete',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'Delete project',exact:true}).click();
  await page.getByText('Renamed fixture',{exact:true}).waitFor({state:'hidden'});assert(projects.length===1,'Delete not submitted');
 });
 await check(`${mode} settings: each tab inventory`,async()=>{
  await page.goto(base+'/settings',{waitUntil:'domcontentloaded'});await page.getByRole('tab').first().waitFor();
  const tabs=page.getByRole('tab');for(let i=0;i<await tabs.count();i++){await tabs.nth(i).click();await inventory(page,'/settings',mode,await tabs.nth(i).innerText());}
 });
 await check(`${mode} scene generator: validation, numeric constraints, cancel`,async()=>{
  await page.goto(base+'/scenes',{waitUntil:'domcontentloaded'});await page.getByRole('button',{name:'New story',exact:true}).first().click();
  const dialog=page.getByRole('dialog');await dialog.waitFor();assert(await dialog.getByRole('button',{name:'Generate script',exact:true}).isDisabled(),'Empty story allowed');
  await dialog.getByLabel('Story idea',{exact:true}).fill('Fixture story');await dialog.getByLabel('Scenes',{exact:true}).fill('21');assert(!await dialog.getByLabel('Scenes',{exact:true}).evaluate(n=>n.checkValidity()),'Scene maximum missing');
  await inventory(page,'/scenes',mode,'story-generator');await dialog.getByRole('button',{name:'Cancel',exact:true}).click();await dialog.waitFor({state:'hidden'});
 });
 await check(`${mode} create and character: empty validation`,async()=>{
  await page.goto(base+'/create',{waitUntil:'domcontentloaded'});await page.getByRole('button',{name:'Create video',exact:true}).waitFor();assert(await page.getByRole('button',{name:'Create video',exact:true}).isDisabled(),'Empty video allowed');
  await page.goto(base+'/characters',{waitUntil:'domcontentloaded'});await page.getByRole('button',{name:'Create character',exact:true}).waitFor();assert(await page.getByRole('button',{name:'Create character',exact:true}).isDisabled(),'Empty character allowed');
 });
 await check(`${mode} theme: light, dark, system and persistence`,async()=>{
  await page.goto(base+'/settings',{waitUntil:'domcontentloaded'});await page.getByRole('tab',{name:'Appearance',exact:true}).click();
  for(const value of ['Light','Dark','System']){const radio=page.getByRole('radio',{name:value,exact:true});await radio.click();assert(await radio.getAttribute('aria-checked')==='true','Theme not selected');assert(await page.evaluate(()=>localStorage.getItem('videoflow.theme'))===value.toLowerCase(),'Theme not persisted');}
 });
 await check(`${mode} backend unavailable: all routes stay rendered`,async()=>{
  missing=true;for(const route of routes){await page.goto(base+route,{waitUntil:'domcontentloaded'});await page.waitForFunction(()=>document.body.innerText.length>20);await page.waitForTimeout(300);assert((await page.locator('body').innerText()).length>20,'Blank failure state '+route);await inventory(page,route,mode,'backend-503');}
 });
 await context.close();
}
}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2));}
console.log(JSON.stringify({checks:report.checks,errors:report.errors,unknownEndpoints:report.unknownEndpoints,report:out+'/report.json'},null,2));
if(report.checks.some(c=>c.status==='fail')||report.errors.length)process.exitCode=1;
