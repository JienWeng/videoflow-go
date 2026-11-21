// Repository Playwright workflow. Only the isolated Go fixture is permitted.
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const base='http://127.0.0.1:8091',api='http://127.0.0.1:18091',out='/tmp/videoflow-creative-presets';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});const report={checks:[],errors:[],axe:[]};
try{for(const width of [1440,390]){
 const mode=width===1440?'desktop':'mobile',context=await browser.newContext({viewport:{width,height:1000}});await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));const page=await context.newPage();page.setDefaultTimeout(15000);page.on('pageerror',e=>report.errors.push(e.message));
 const before=await (await page.request.get(api+'/e2e/counters')).json();assert('images' in before);
 const check=async(name,fn)=>{await fn();report.checks.push({id:`PRESETS-${mode}-${report.checks.length+1}`,name,status:'pass'});console.log('PASS',mode,name)};
 let pack;
 await check('six built-ins and portable starter',async()=>{const r=await page.request.get(api+'/preset-packs/starter/export');assert(r.ok());pack=await r.json();assert.equal(pack.presets.length,6);pack.id='browser-'+mode;pack.name='Browser '+mode;pack.presets=pack.presets.slice(2,3);pack.presets[0].name='Community clay '+mode;const c=await(await page.request.get(api+'/style-presets')).json();const existing=c.packs.find(p=>p.pack.id===pack.id);if(existing){const removed=await page.request.delete(api+'/preset-packs/'+pack.id,{headers:{'If-Match':existing.hash}});assert(removed.ok())}});
 await page.goto(base+'/settings#presets');await page.getByRole('heading',{name:'Creative presets',exact:true}).waitFor();
 await check('invalid JSON shows recoverable error',async()=>{await page.getByLabel('Preview a preset pack').setInputFiles({name:'bad.json',mimeType:'application/json',buffer:Buffer.from('{bad')});await page.getByRole('alert').filter({hasText:'SyntaxError'}).waitFor()});
 await check('preview does not install',async()=>{await page.getByLabel('Preview a preset pack').setInputFiles({name:'community.videoflow-presets.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(pack))});await page.getByRole('button',{name:'Install pack',exact:true}).waitFor();const c=await(await page.request.get(api+'/style-presets')).json();assert(!c.packs.some(p=>p.pack.id===pack.id));await page.getByText(pack.presets[0].name,{exact:true}).click();await page.getByText(pack.presets[0].style_prompt,{exact:true}).last().waitFor()});
 await check('install adds namespaced choice',async()=>{await page.getByRole('button',{name:'Install pack',exact:true}).click();await page.getByRole('status').filter({hasText:'Installed Browser'}).waitFor();const c=await(await page.request.get(api+'/style-presets')).json();assert(c.presets.some(p=>p.id===`community:${pack.id}:${pack.presets[0].id}`))});
 await check('installed pack exports valid JSON',async()=>{const r=await page.request.get(api+`/preset-packs/${pack.id}/export`);assert.deepEqual(await r.json(),pack)});
 await check('same pack is idempotent',async()=>{await page.getByLabel('Preview a preset pack').setInputFiles({name:'same.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(pack))});assert(await page.getByRole('button',{name:'Already installed'}).isDisabled());await page.getByRole('button',{name:'Cancel',exact:true}).click()});
 await check('replacement requires explicit reviewed revision',async()=>{pack.version='1.1.0';pack.presets[0].guidance.video+=' Preserve paper-free clay textures.';const bad=await page.request.post(api+'/preset-packs/import',{data:pack});assert.equal(bad.status(),409);await page.getByLabel('Preview a preset pack').setInputFiles({name:'replacement.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(pack))});await page.getByRole('button',{name:'Replace pack'}).click();await page.getByText('Browser '+mode+' · 1.1.0',{exact:true}).waitFor()});
 const audit=async(name)=>{const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();report.axe.push({mode,name,violations:result.violations});assert.equal(result.violations.length,0,JSON.stringify(result.violations))};
 await audit('preset settings');await page.screenshot({path:`${out}/${mode}-settings.png`,fullPage:true});
 await page.goto(base+'/create');
 await check('new styles and community preset selectable',async()=>{const select=page.getByRole('combobox',{name:'Visual style',exact:true});await select.selectOption(`community:${pack.id}:${pack.presets[0].id}`);assert.equal(await select.inputValue(),`community:${pack.id}:${pack.presets[0].id}`);await page.getByText('About Community clay '+mode,{exact:true}).click();await page.getByText(pack.presets[0].description,{exact:true}).waitFor();for(const name of ['Layered paper-cut','Graphic novel','Soft 3D animation'])assert(await select.getByRole('option',{name,exact:true}).count())});
 await audit('story preset selection');await page.screenshot({path:`${out}/${mode}-story.png`,fullPage:true});
 await check('mobile content fits viewport',async()=>assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)));
 await page.goto(base+'/settings#presets');
 await check('remove pack clears catalog',async()=>{page.once('dialog',d=>d.accept());await page.getByRole('listitem').filter({hasText:pack.name}).getByRole('button',{name:'Remove',exact:true}).click();await page.getByRole('status').filter({hasText:'Removed Browser'}).waitFor();const c=await(await page.request.get(api+'/style-presets')).json();assert(!c.packs.some(p=>p.pack.id===pack.id))});
 await check('preset operations never call providers',async()=>assert.deepEqual(await(await page.request.get(api+'/e2e/counters')).json(),before));
 await context.close();
 }assert.equal(report.errors.length,0);console.log(JSON.stringify({checks:report.checks.length,axe:report.axe.length,errors:report.errors.length}));
}finally{await writeFile(out+'/report.json',JSON.stringify(report,null,2));await browser.close()}
