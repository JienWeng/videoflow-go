import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:8091',out='/tmp/videoflow-contextual-workflow';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});
const report={checks:[],errors:[],axe:[]};
try{for(const mobile of [false,true]){
 const mode=mobile?'mobile':'desktop',context=await browser.newContext({viewport:mobile?{width:390,height:844}:{width:1440,height:1000}});
 await context.addInitScript(()=>{localStorage.setItem('videoflow.onboarded','1');localStorage.setItem('videoflow.canvas.coach','1')});
 let withOutput=false;
 const characters=[{id:'pip',name:'Pip',description:'A puppy',reference_asset_ids_json:[]},{id:'mama',name:'Mama',reference_asset_ids_json:[]}];
 await context.route('**/*',async r=>{const u=new URL(r.request().url()),path=u.pathname.replace(/^\/api/,'');if(!u.pathname.startsWith('/api/'))return u.origin===new URL(base).origin?r.continue():r.abort();if(r.request().method()!=='GET')throw Error('Unexpected write '+path);let body=[];
 if(path==='/events')return r.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
 if(path==='/projects/active')body={id:'p',name:'Workflow film'};
 else if(path==='/characters')body=characters;
 else if(path==='/voice/state')body={project_id:'p',profiles:[],scripts:[],tasks:withOutput?[{id:'task',kind:'speech',model:'test',status:'draft',fingerprint:'fixture',bindings:[]}]:[]};
 else if(path==='/assets/types')body={types:['other']};
 else if(path==='/style')body={};
 else if(path==='/graph')body={nodes:[],edges:[]};
 else if(path==='/director/status')body={engines:[{engine:'codex',installed:true}],all_text_local:true};
 else if(path==='/director/sessions')body=[{id:'session',title:'Story planning',engine:'codex',selected:true,status:'ready'}];
 else if(path==='/director/sessions/session/selection')body={character_ids:[],asset_ids:[]};
 else if(path==='/render-jobs')body=withOutput?[{id:'job',status:'succeeded',scene_id:'scene',model:'test'}]:[];
 else if(path==='/render-jobs/job')body={job:{id:'job',model:'test'},outputs:[{id:'out',video_path:'storage/test.mp4'}]};
 await r.fulfill({json:body});});
 const p=await context.newPage();p.on('pageerror',e=>report.errors.push(mode+': '+e.message));
 async function check(name,fn){await fn();report.checks.push({auditId:`FLOW-${mode.toUpperCase()}-${report.checks.length+1}`,name,status:'pass'});console.log('PASS',mode,name)}
 await check('sidebar has one Library and no miscellaneous tools menu',async()=>{await p.goto(base+'/library');await p.getByRole('heading',{name:'Library',exact:true}).waitFor();assert.equal(await p.getByText('More tools',{exact:true}).count(),0);assert.equal(await p.locator('aside a[href="/library"]').count(),1)});
 await context.close();
}}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2))}
assert.equal(report.errors.length,0,JSON.stringify(report.errors));console.log('PASS',report.checks.length,'workflow checks;',report.axe.length,'accessibility scans');
