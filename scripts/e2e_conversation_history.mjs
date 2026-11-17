// Isolated browser regression: all API requests intercepted, no AI/provider calls.
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const out='/tmp/videoflow-history';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});
const report={checks:[],errors:[],axe:[]};
try{for(const width of [1440,390]){
 const context=await browser.newContext({viewport:{width,height:900},reducedMotion:'reduce'});await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));
 let selected='new',writes=[],failOld=false;
 const turns=id=>[{kind:'chat',request_json:{user_message:id==='old'?'The puppy follows pawprints':'A new story'},result_json:{reply:id==='old'?'He finds his mother beneath the bridge':'Let’s plan it.'}}];
 await context.route('**/api/**',async r=>{const p=new URL(r.request().url()).pathname.slice(4);let body=[];
 if(r.request().method()!=='GET'){writes.push(p);if(p==='/director/sessions/old/select'){selected='old';return r.fulfill({json:{id:'old'}})}return r.fulfill({status:409,json:{error:'Unexpected write'}})}
 if(p==='/events')return r.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
 if(p==='/director/status')body={all_text_local:true,engines:[{engine:'codex',installed:true}]};
 else if(p==='/director/sessions')body=['new','old'].map(id=>({id,title:id==='new'?'New story':'Pawprints to Mama',engine:'codex',selected:selected===id,status:'ready'}));
 else if(p.endsWith('/turns')){if(p.includes('/old/')&&failOld)return r.fulfill({status:503,json:{error:'History offline'}});body=turns(p.includes('/old/')?'old':'new')}
 else if(p==='/graph')body={nodes:[],edges:[]};else if(p==='/projects/active')body={id:'project',name:'Pawprints'};else if(p==='/chat/capabilities')body={subagents:{supported:false}};
 return r.fulfill({json:body});});
 const page=await context.newPage();page.setDefaultTimeout(8000);page.on('pageerror',e=>report.errors.push(e.message));await page.goto((process.env.E2E_BASE_URL||'http://127.0.0.1:8091')+'/?view=studio');if(width===390)await page.getByRole('button',{name:/Workbench/}).click();
 await page.getByText('Let’s plan it.',{exact:true}).waitFor();report.checks.push(`${width}: current saved chat restores`);
 await page.getByRole('textbox',{name:'Message story assistant'}).fill('Unsent direction');await page.getByRole('button',{name:'Conversation history',exact:true}).click();const d=page.getByRole('dialog',{name:'Conversation history',exact:true});await d.getByText('A new story',{exact:true}).waitFor();await d.getByRole('combobox',{name:'Saved conversation'}).selectOption('old');await d.getByText('He finds his mother beneath the bridge',{exact:true}).waitFor();assert.equal(writes.length,0);report.checks.push(`${width}: browsing old transcript is read only`);
 await page.waitForTimeout(350);const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();report.axe.push({width,violations:axe.violations});assert.deepEqual(axe.violations.map(v=>v.id),[]);await page.screenshot({path:`${out}/history-${width}.png`});
 await d.getByRole('button',{name:'Open in chat',exact:true}).click();await d.waitFor({state:'hidden'});await page.getByRole('log',{name:'Story conversation'}).getByText('He finds his mother beneath the bridge',{exact:true}).waitFor();assert.deepEqual(writes,['/director/sessions/old/select']);assert.equal(await page.getByRole('textbox',{name:'Message story assistant'}).inputValue(),'Unsent direction');report.checks.push(`${width}: explicit open restores transcript, retains draft and replays no generation`);
 await page.getByRole('button',{name:'Conversation history',exact:true}).click();await d.getByText('He finds his mother beneath the bridge',{exact:true}).waitFor();failOld=true;await d.getByRole('combobox',{name:'Saved conversation'}).selectOption('new');await d.getByText('A new story',{exact:true}).waitFor();await d.getByRole('combobox',{name:'Saved conversation'}).selectOption('old');await d.getByRole('alert').waitFor();assert.equal(await d.getByRole('button',{name:'Open in chat',exact:true}).isEnabled(),false);failOld=false;await d.getByRole('button',{name:'Retry history'}).click();await d.getByText('He finds his mother beneath the bridge',{exact:true}).waitFor();await page.keyboard.press('Escape');assert.equal(writes.length,1);report.checks.push(`${width}: history failure retries without switching or replaying`);await context.close();
}}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2))}
assert.equal(report.errors.length,0);console.log(JSON.stringify(report));
