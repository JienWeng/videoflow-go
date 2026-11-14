// Requires the isolated Go E2E fixture on 18091 and its frontend on 5195.
import {mkdir,writeFile,readFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:5195',api=process.env.E2E_API_URL||'http://127.0.0.1:18091',out=process.env.E2E_OUTPUT||'/tmp/videoflow-transfer-e2e';
await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
const page=await browser.newPage();page.setDefaultTimeout(15000);await page.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));
const report={checks:[],errors:[]};page.on('pageerror',e=>report.errors.push(e.message));
const get=async p=>{const r=await page.request.get(api+p);assert(r.ok(),p);return r.json()};
async function check(name,fn){await fn();report.checks.push({name,status:'pass'});console.log('PASS',name)}
try{
 await get('/e2e/counters'); // Refuse to mutate a production server.
 const active=await get('/projects/active'),before=await get('/projects');let bundle;
 await check('Export UI downloads valid project JSON with existing media references',async()=>{
  await page.goto(base+'/projects');const download=page.waitForEvent('download');await page.getByRole('button',{name:'Export',exact:true}).first().click();const d=await download;assert.equal(await d.failure(),null);await d.saveAs(out+'/project.json');bundle=JSON.parse(await readFile(out+'/project.json','utf8'));assert(bundle.characters.length>0);assert(bundle.assets.length>0);assert(bundle.assets.every(a=>a.file_path));
 });
 await check('Import UI round-trips data and remaps bindings without switching active project',async()=>{
  await page.locator('input[type=file]').setInputFiles(out+'/project.json');await page.getByText('Project imported',{exact:true}).waitFor();const projects=await get('/projects');assert.equal(projects.length,before.length+1);const copied=projects.find(p=>!before.some(b=>b.id===p.id));assert(copied);const exported=await get('/projects/'+copied.id+'/export');assert.equal(exported.characters.length,bundle.characters.length);assert.equal(exported.assets.length,bundle.assets.length);assert.notEqual(exported.characters[0].id,bundle.characters[0].id);assert.equal((await get('/projects/active')).id,active.id);for(const a of exported.assets){if(a.character_id)assert(exported.characters.some(c=>c.id===a.character_id));const r=await page.request.get(api.replace(/\/$/,'')+'/'+a.file_path.slice(a.file_path.indexOf('storage/')));assert(r.ok(),'Copied media no longer resolves');}
 });
 await check('ZIP and malformed JSON rejected without adding projects; input can retry',async()=>{
  const count=(await get('/projects')).length;for(const [name,text] of [['broken.zip','PK invalid zip'],['broken.json','{']]){await page.locator('input[type=file]').setInputFiles({name,mimeType:'application/octet-stream',buffer:Buffer.from(text)});await page.getByText(/ZIP files are not supported/).first().waitFor();assert.equal((await get('/projects')).length,count);assert.equal(await page.locator('input[type=file]').inputValue(),'');}
 });
 await check('Missing media and external paths reject atomically with actionable error',async()=>{
  const count=(await get('/projects')).length;for(const path of ['storage/assets/missing-file.png','/etc/passwd']){const bad=structuredClone(bundle);bad.assets[0].file_path=path;const r=await page.request.post(api+'/projects/import',{data:bad});assert.equal(r.status(),400);assert.match(await r.text(),/media|storage/i);assert.equal((await get('/projects')).length,count);}
 });
}catch(e){report.failure=e.stack;process.exitCode=1;console.error(e)}finally{assert.equal(report.errors.length,0);await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2))}
