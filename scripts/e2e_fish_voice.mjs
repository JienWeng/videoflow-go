// Repository browser fixture workflow. All keys, provider calls and speech are mocks.
import assert from 'node:assert/strict';import{mkdir,writeFile}from'node:fs/promises';
const{chromium}=await import('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs');
const{default:AxeBuilder}=await import('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs');
const out='/tmp/videoflow-fish-voice';await mkdir(out,{recursive:true});const report={checks:[],errors:[],axe:[]};
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:8091';
try{for(const width of[1440,390]){
 const context=await browser.newContext({viewport:{width,height:1000},reducedMotion:'reduce'});await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>report.errors.push(e.message));
 let configured=false,profile=null,tasks=[],runs=0;
 await page.route('**/api/**',async route=>{
 const request=route.request(),p=new URL(request.url()).pathname.replace('/api',''),method=request.method();let body={};
 if(p==='/settings/providers')body=[{name:'fish',label:'Fish Audio',protocol:'speech',configured,models:['s2.1-pro']}];
 else if(p==='/settings/providers/fish'){if(method==='PUT'){assert.equal(request.postDataJSON().api_key,'fixture-fish-key');configured=true}body={name:'fish',configured,from_db:configured,masked_key:configured?'••••key':'',base_url:'https://api.fish.audio'}}
 else if(p==='/settings/providers/fish/test')body={ok:configured,checked:'configuration',credentials_verified:false,generation_tested:false};
 else if(p==='/settings/app')body={all_text_local:true,default_aspect_ratio:'16:9',default_scene_duration:5,dialogue_language:'English',caption_style:'clean',caption_language:'auto',render_negatives:[],media_model_preferences:{}};
 else if(p==='/voice/state')body={project_id:'p',profiles:profile?[{id:'c',revision:1,data:profile}]:[],scripts:[{id:'main',revision:1,data:{approved:true,lines:[{id:'line',character_id:'c',language:'en',text:'Come home, Pip!',visible:true}]}}],tasks};
 else if(p==='/voice/profiles/c'){profile=request.postDataJSON().data;assert.equal(profile.provider,'fish');assert.equal(profile.voice_id,'fish-voice-id');assert.equal(profile.model,'s2.1-pro-free');body={id:'c',revision:1,data:profile}}
 else if(p==='/voice/tasks/prepare'){assert.equal(request.postDataJSON().kind,'speech');tasks=[{id:'task',kind:'speech',model:'fish/s2.1-pro-free',status:'draft',fingerprint:'exact',bindings:[]}];body=tasks[0]}
 else if(p==='/voice/tasks/task/run'){assert.equal(request.postDataJSON().fingerprint,'exact');runs++;tasks[0]={...tasks[0],status:'completed',result:{asset_id:'speech',local_path:'storage/assets/speech.mp3'}};body=tasks[0]}
 else if(p==='/projects/active')body={id:'p',name:'Fish voice QA'};
 else if(p==='/characters')body=[{id:'c',name:'Shopkeeper'}];
 else if(['/projects','/settings/agents','/assets','/scenes','/render-jobs','/ops','/director/sessions'].includes(p))body=[];
 else if(p==='/events')return route.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
 await route.fulfill({json:body});
 });
 await page.goto(base+'/settings#connections');await page.locator('summary').filter({hasText:'Fish Audio'}).click();await page.locator('#key-fish').fill('fixture-fish-key');await page.getByRole('button',{name:'Save',exact:true}).click();await page.getByRole('button',{name:'Check configuration',exact:true}).click();await page.getByText('Configuration checked',{exact:true}).waitFor();assert(configured);assert.equal(runs,0);assert.equal(await page.locator('[data-provider="fish"]').getByRole('button',{name:'Verify',exact:true}).count(),0);report.checks.push(`${width}: Fish key configuration saved, no paid model verification or generation`);
 await page.goto(base+'/voices?character_id=c');await page.getByLabel('Speech provider',{exact:true}).selectOption('fish');await page.getByLabel('Speech model',{exact:true}).selectOption('s2.1-pro-free');await page.getByLabel('Provider voice ID',{exact:true}).fill('fish-voice-id');await page.getByRole('checkbox',{name:"I approve this character's sample / provider voice ID."}).check();await page.getByRole('button',{name:'Save voice revision'}).click();await page.getByText('Character voice revision saved.',{exact:false}).waitFor();assert(profile);await page.reload();await page.getByLabel('Speech provider',{exact:true}).waitFor();assert.equal(await page.getByLabel('Speech provider',{exact:true}).inputValue(),'fish');assert.equal(await page.getByLabel('Speech model',{exact:true}).inputValue(),'s2.1-pro-free');assert.equal(await page.getByLabel('Provider voice ID',{exact:true}).inputValue(),'fish-voice-id');assert.equal(await page.getByRole('button',{name:'Prepare voice creation'}).count(),0);report.checks.push(`${width}: character-specific Fish provider, voice ID and model saved without cloning`);
 await page.getByRole('button',{name:'Prepare saved-voice speech'}).click();const run=page.getByRole('button',{name:'Run paid speech'});await run.waitFor();assert.equal(runs,0);await run.click();await page.getByRole('link',{name:'Open generated speech'}).waitFor();assert.equal(runs,1);report.checks.push(`${width}: speech prepares without payment, explicit Run completes with a reusable audio result`);
 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();report.axe.push({width,violations:axe.violations});assert.equal(axe.violations.length,0);await page.screenshot({path:`${out}/fish-${width}.png`});await context.close();
}}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2))}
assert.equal(report.errors.length,0);console.log(JSON.stringify({checks:report.checks.length,axe:report.axe.length,errors:report.errors}));
