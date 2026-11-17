// Creative brief editing and multi-turn persistence. Providers are fully mocked.
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
import {writeFile} from 'node:fs/promises';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});
const checks=[];
try {for(const width of [1440,390]){
 const context=await browser.newContext({viewport:{width,height:1000}}),page=await context.newPage();page.setDefaultTimeout(15000);
 await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));
 const requests=[],generation=[],history=[];let draft={idea:'A puppy finds Mama'};
 await context.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname.replace('/api','');let body={};
 if(path==='/events')return route.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
 if(path==='/projects/active')body={id:'qa',name:'Brief project'};
 else if(path==='/director/sessions')body=[{id:'qa',selected:true,title:'Story planning',engine:'codex',status:'ready'}];
 else if(path==='/director/status')body={all_text_local:true,engines:[]};
 else if(path==='/chat/capabilities')body={local:true,subagents:{supported:false}};
 else if(path==='/graph')body={nodes:[],edges:[]};
 else if(path==='/director/sessions/qa/turns')body=history;
 else if(['/projects','/characters','/scenes','/assets','/ops'].includes(path))body=[];
 else if(path==='/style-presets')body={presets:[{id:'2d-picture-book',name:'Picture book',description:'Warm illustrations with expressive faces.',fingerprint:'a'.repeat(64)},{id:'clay-stop-motion',name:'Clay stop-motion',description:'Tactile miniature sets.',fingerprint:'b'.repeat(64)}]};
 else if(path==='/chat'){const data=req.postDataJSON();requests.push(data);draft={...draft,...data.creation_brief};body={intent:{action:'generate_script',reply:'Confirm the choices below before creating.',idea:draft.idea,creation_brief:draft},options:{characters:[],scenes:[],outputs:[]},state:{}};history.push({kind:'intent_agent',request_json:{user_message:data.message},result_json:{reply:body.intent.reply}});}
 else if(['/videos/generate','/scripts/generate'].includes(path)){generation.push(req.postDataJSON());return route.fulfill({status:400,body:'Fixture stops before generation'});}
 await route.fulfill({json:body});
 });
 const open=async()=>{await page.goto('http://127.0.0.1:8091/?view=studio');if(width===390)await page.getByRole('button',{name:/Workbench/}).click();};
 await open();const composer=()=>page.getByRole('textbox',{name:'Message story assistant',exact:true});
 await composer().fill(draft.idea);await composer().press('Enter');
 await page.getByRole('button',{name:/Picture book.*Choose style/}).click();await page.getByLabel('Tone',{exact:true}).fill('Hopeful');await page.getByLabel('Audience',{exact:true}).fill('Families');
 await page.getByRole('button',{name:'Continue',exact:true}).click();
 await page.getByLabel('Destination platform',{exact:true}).fill('YouTube Shorts');await page.getByRole('combobox',{name:'Format',exact:true}).selectOption('9:16');await page.getByRole('combobox',{name:'Video size',exact:true}).selectOption('720p');await page.getByLabel('Spoken language',{exact:true}).fill('Malay');await page.getByRole('combobox',{name:'Speech',exact:true}).selectOption('narration');
 await page.getByRole('button',{name:'Continue',exact:true}).click();await page.getByLabel('Total video duration (seconds)',{exact:true}).fill('12');await page.getByLabel('Number of scenes',{exact:true}).fill('2');
 await page.getByRole('button',{name:'Continue',exact:true}).click();assert(await page.getByRole('button',{name:'Create story & references',exact:true}).isDisabled());
 await page.getByLabel('These are the settings I want to use',{exact:true}).check();
 await page.getByRole('button',{name:'1. Look & mood',exact:true}).click();await page.getByLabel('Tone',{exact:true}).fill('Gentle and hopeful');assert(await page.getByRole('button',{name:'Create story & references',exact:true}).isDisabled());
 await composer().fill('Keep these choices');await composer().press('Enter');await page.getByText('Earlier creative brief · updated below',{exact:true}).waitFor();
 assert.equal(requests.at(-1).creation_brief.idea,'A puppy finds Mama');assert.equal(requests.at(-1).creation_brief.scene_count,2);assert.equal(requests.at(-1).creation_brief.tone,'Gentle and hopeful');
 assert.equal(await page.getByRole('button',{name:'Create story & references',exact:true}).count(),1);
 await page.getByRole('button',{name:'4. Confirm',exact:true}).click();await page.getByText('12 seconds',{exact:true}).waitFor();
 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();assert.deepEqual(axe.violations,[]);
 await open();await page.getByRole('button',{name:'4. Confirm',exact:true}).click();await page.getByText('Gentle and hopeful',{exact:true}).waitFor();assert(await page.getByRole('button',{name:'Create story & references',exact:true}).isDisabled());
 assert.equal(generation.length,0);await page.getByLabel('These are the settings I want to use',{exact:true}).check();await page.getByRole('button',{name:'Create story & references',exact:true}).click();await page.waitForFunction(()=>document.body.innerText.includes('Fixture stops before generation'));
 assert.equal(generation.length,1);assert.equal(generation[0].scene_count,2);assert.equal(generation[0].target_duration,12);assert.equal(generation[0].language,'Malay');assert.equal(generation[0].brief_confirmed,true);assert.equal(generation[0].brief_version,1);assert.equal(generation[0].resolution,'720p');assert.equal(generation[0].tone,'Gentle and hopeful');
 checks.push({width,checks:['visible style selection','all delivery and timing fields','edit invalidates confirmation','multi-turn edited context','one active card','reload persistence with fresh confirmation','complete generation payload','zero axe violations']});console.log('PASS creative brief',width);await context.close();
}}finally{await writeFile('/tmp/creative-brief-browser-results.json',JSON.stringify(checks,null,2));await browser.close();}
