// Real isolated Go router and production UI. No live AI/provider calls.
import assert from 'node:assert/strict';import {readFile,mkdir,writeFile} from 'node:fs/promises';import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const base='http://127.0.0.1:8091',api='http://127.0.0.1:18091',out='/tmp/videoflow-node-editing';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});const report={checks:[],axe:[],errors:[]};
try{for(const width of [1440,390]){
 const mode=width===1440?'desktop':'mobile',context=await browser.newContext({viewport:{width,height:1000}});await context.addInitScript(()=>{localStorage.setItem('videoflow.onboarded','1');localStorage.setItem('videoflow.canvas.coach','1')});const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>report.errors.push({mode,error:e.message}));
 const read=async p=>{const r=await page.request.get(api+p);assert(r.ok(),await r.text());return r.json()};const before=await read('/e2e/counters');assert('videos' in before);
 const check=async(name,fn)=>{await fn();report.checks.push({id:`NODE-EDIT-${mode}-${report.checks.length+1}`,name,status:'pass'});console.log('PASS',mode,name)};
 const r=await page.request.post(api+'/assets/upload',{multipart:{type:'video',file:{name:mode+'-edit.mp4',mimeType:'video/mp4',buffer:await readFile('/tmp/vf-edit-video.mp4')}}});assert(r.ok(),await r.text());const asset=await r.json();let saved;
 await check('source metadata is local and project-scoped',async()=>{const source=await read('/compositions/sources/'+asset.id);assert.equal(source.duration,3);assert(source.source.url.includes('storage/'));const missing=await page.request.get(api+'/compositions/sources/not-owned');assert.equal(missing.status(),404)});
 await page.goto(base+'/');
 if(width===1440){
 await check('asset node exposes full library and media editing',async()=>{await page.locator('.svelte-flow__node').filter({hasText:asset.name}).first().click();await page.getByRole('link',{name:'Open asset library',exact:true}).waitFor();assert((await page.getByRole('link',{name:'Trim, frame & arrange media',exact:true}).getAttribute('href')).endsWith(asset.id));await page.getByLabel('Asset name',{exact:true}).fill('Node edited '+mode);await page.getByRole('button',{name:'Save asset details',exact:true}).click();await page.getByText('Saved',{exact:true}).waitFor();assert.equal((await read('/assets/'+asset.id)).name,'Node edited '+mode)});
 await check('character node uses canonical library tools',async()=>{await page.locator('.svelte-flow__node').filter({hasText:'Mimi'}).first().click();await page.getByRole('link',{name:'Open character library',exact:true}).waitFor();await page.getByRole('link',{name:'Voice & dialogue',exact:true}).waitFor()});
 }
 await check('manual editing does not call AI providers',async()=>assert.deepEqual(await read('/e2e/counters'),before));await context.close();
 }assert.equal(report.errors.length,0,JSON.stringify(report.errors));console.log(JSON.stringify({checks:report.checks.length,axe:report.axe.length,errors:report.errors.length}));
}finally{await writeFile(out+'/report.json',JSON.stringify(report,null,2));await browser.close()}
