// Complete mocked project management flows; no live project writes or providers.
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
import {mkdir,writeFile} from 'node:fs/promises';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:8091';
const out='/tmp/videoflow-project-browser';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',args:['--no-sandbox'],headless:true});
const report=[];
try{for(const width of (process.env.E2E_WIDTH?[Number(process.env.E2E_WIDTH)]:[1440,390])){
 const ctx=await browser.newContext({viewport:{width,height:1000},acceptDownloads:true});await ctx.addInitScript(()=>{localStorage.setItem('videoflow.onboarded','1')});
 let projects=[{id:'puppy',name:'Pawprints to Mama',description:'A lost puppy returns home',is_active:true,created_at:'2026-11-16',updated_at:'2026-11-18',counts:{scenes:2,characters:3,assets:5}},{id:'alien',name:'Alien visits Earth',description:'A curious guide discovers cities',is_active:false,created_at:'2026-11-17',updated_at:'2026-11-17',counts:{scenes:4,characters:1,assets:6}}];
 let failCreate=true,failActivate=false,failList=false,creates=0,activations=0;const writes=[],errors=[];
 await ctx.route('**/api/**',async route=>{const req=route.request(),path=new URL(req.url()).pathname.slice(4),method=req.method();let body={};if(path==='/events')return route.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
 if(method!=='GET')writes.push({path,method});
 if(path==='/projects'&&method==='GET'){if(failList)return route.fulfill({status:503,body:'Fixture list unavailable'});body=projects}
 else if(path==='/projects'&&method==='POST'){creates++;if(failCreate)return route.fulfill({status:503,body:'Fixture creation failed'});const data=req.postDataJSON();body={...data,id:'new',created_at:'2026-11-18',updated_at:'2026-11-18',is_active:false,counts:{}};projects.push(body)}
 else if(path.endsWith('/activate')){activations++;if(failActivate)return route.fulfill({status:503,body:'Fixture activation failed'});const id=path.split('/')[2];projects=projects.map(p=>({...p,is_active:p.id===id}));body={status:'ok'}}
 else if(method==='PATCH'&&path.startsWith('/projects/')){const p=projects.find(p=>p.id===path.split('/')[2]);Object.assign(p,req.postDataJSON());body=p}
 else if(method==='DELETE'&&path.startsWith('/projects/')){projects=projects.filter(p=>p.id!==path.split('/')[2]);if(!projects.some(p=>p.is_active)&&projects.length)projects[0].is_active=true;body={status:'deleted'}}
 else if(path==='/projects/import'){projects.push({id:'import',name:'Imported story',is_active:false,created_at:'2026-11-18',counts:{}});body={id:'import'}}
 else if(path.endsWith('/export'))body={project:projects.find(p=>p.id===path.split('/')[2])};
 else if(path==='/projects/active')body=projects.find(p=>p.is_active);
 else if(path==='/style')body={project_id:projects.find(p=>p.is_active)?.id,name:'Picture book',style_prompt:'warm ink',palette:'warm',lighting:'soft',tone:'Kind',audience:'Families'};
 else if(path==='/style-presets')body={presets:[{id:'picture-book',name:'Picture book',description:'Warm ink illustration',style_prompt:'warm ink',guidance:{story:'Readable goals'}}],has_project_style:true};
 else if(['/characters','/assets','/scenes','/ops','/director/sessions'].includes(path))body=[];
 await route.fulfill({json:body});});
 const page=await ctx.newPage();page.setDefaultTimeout(12000);page.on('pageerror',e=>errors.push(String(e)));
 const axe=async name=>{await page.evaluate(()=>Promise.all(document.getAnimations().filter(a=>a.effect?.getTiming().iterations!==Infinity).map(a=>a.finished.catch(()=>{}))));const result=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();assert.deepEqual(result.violations,[],name);const overflow=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,items:[...document.querySelectorAll('body *')].filter(e=>e.getBoundingClientRect().right>innerWidth+1).slice(0,12).map(e=>({tag:e.tagName,cls:e.className,right:e.getBoundingClientRect().right,width:e.getBoundingClientRect().width}))}));if(overflow.scroll>overflow.width)console.log(JSON.stringify(overflow));assert.equal(overflow.scroll>overflow.width,false,name+' page overflow')};
 await page.goto(base+'/projects');await page.getByRole('region',{name:'Project folders'}).getByRole('link',{name:'Pawprints to Mama'}).waitFor();assert.equal(await page.getByLabel('Project name',{exact:true}).count(),0);assert.equal(await page.getByRole('heading',{name:/Visual style/}).count(),0);await axe('folder view');await page.screenshot({path:`${out}/folders-${width}.png`,fullPage:true});
 await page.getByRole('button',{name:'Table view',exact:true}).click();const table=page.getByRole('table');assert.equal(await table.getByRole('row').count(),3);
 const puppyRow = table.locator('tbody tr').filter({hasText:'Pawprints to Mama'});
 assert.deepEqual((await puppyRow.getByRole('cell').allTextContents()).slice(0, 3), ['2', '3', '5']);
 assert.equal(await puppyRow.getByRole('link').getAttribute('href'), '/projects/detail?id=puppy');
 assert.equal(await table.getByRole('columnheader').filter({hasText:'Scenes'}).count(),1);await page.getByRole('searchbox',{name:'Search projects'}).fill('curious');assert.equal(await table.getByRole('row').count(),2);await page.getByRole('searchbox',{name:'Search projects'}).fill('');await page.getByLabel('Sort projects').selectOption('name');assert.equal(await table.getByRole('link').first().innerText(),'Alien visits Earth');await axe('table view');await page.screenshot({path:`${out}/table-${width}.png`,fullPage:true});await page.reload();await page.getByRole('table').waitFor();assert.equal(await page.getByRole('button',{name:'Table view',exact:true}).getAttribute('aria-pressed'),'true');
 // Visiting a project detail never activates it or fetches another project's style.
 await page.getByRole('link',{name:'Alien visits Earth',exact:true}).click();await page.getByRole('heading',{name:'Alien visits Earth',exact:true}).waitFor();assert.equal(activations,0);
 assert.deepEqual(await page.locator('main dl dd').allTextContents(), ['4', '1', '6']);
 assert.equal(await page.getByRole('heading',{name:/Visual style/}).count(),0);await page.getByRole('button',{name:'Activate to edit style'}).click();await page.getByRole('heading',{name:'Visual style · Alien visits Earth'}).waitFor();assert.equal(activations,1);await axe('project detail');await page.screenshot({path:`${out}/detail-${width}.png`,fullPage:true});
 await page.getByRole('link',{name:'All projects',exact:true}).click();await page.getByRole('button',{name:'New project',exact:true}).click();const dialog=page.getByRole('dialog',{name:'New project',exact:true});await dialog.getByLabel('Project name',{exact:true}).fill('New dog story');await dialog.getByLabel('Description (optional)').fill('A hopeful reunion');await axe('create dialog');await dialog.getByRole('button',{name:'Create project',exact:true}).click();await dialog.getByRole('alert').filter({hasText:'Fixture creation failed'}).waitFor();assert.equal(await dialog.getByLabel('Project name').inputValue(),'New dog story');
 // Cancelling a dirty dialog can be declined, preserving the draft.
 page.once('dialog',d=>d.dismiss());await page.keyboard.press('Escape');assert.equal(await dialog.isVisible(),true);
 failCreate=false;failActivate=true;await dialog.getByRole('button',{name:'Create project',exact:true}).click();await dialog.getByRole('alert').filter({hasText:'Project created.'}).waitFor();assert.equal(creates,2);failActivate=false;await dialog.getByRole('button',{name:'Retry opening project',exact:true}).click();await page.getByRole('heading',{name:'New dog story',exact:true}).waitFor();assert.equal(creates,2,'activation retry must not create again');assert.equal(new URL(page.url()).pathname,'/projects/detail');assert.equal(new URL(page.url()).searchParams.get('id'),'new');
 await page.getByRole('link',{name:'All projects',exact:true}).click();await page.getByRole('button',{name:'Actions for Pawprints to Mama'}).click();await page.getByRole('menuitem',{name:'Rename',exact:true}).click();const rename=page.getByRole('dialog',{name:'Rename project',exact:true});await rename.getByLabel('Project name').fill('Pip comes home');await rename.getByRole('button',{name:'Save name'}).click();await page.getByRole('link',{name:'Pip comes home',exact:true}).waitFor();
 await page.getByRole('button',{name:'Actions for Pip comes home'}).click();const downloaded=page.waitForEvent('download');await page.getByRole('menuitem',{name:'Export project',exact:true}).click();assert.equal((await downloaded).suggestedFilename(),'Pip_comes_home.json');
 await page.locator('input[type=file]').setInputFiles({name:'project.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify({project:{name:'Imported story'}}))});await page.getByRole('link',{name:'Imported story',exact:true}).waitFor();
 await page.getByRole('button',{name:'Actions for Imported story'}).click();await page.getByRole('menuitem',{name:'Delete project',exact:true}).click();const deletion=page.getByRole('dialog',{name:'Delete “Imported story”?',exact:true});await deletion.getByRole('button',{name:'Delete project',exact:true}).click();await deletion.waitFor({state:'hidden'});await page.getByRole('link',{name:'Imported story',exact:true}).waitFor({state:'hidden'});
 failList=true;await page.reload();await page.getByRole('alert').filter({hasText:'Fixture list unavailable'}).waitFor();failList=false;await page.getByRole('button',{name:'Try again',exact:true}).click();await page.locator('a[href="/projects/detail?id=new"]').waitFor();await page.locator('a[href="/projects/detail?id=new"]').first().click();
 await page.getByRole('button',{name:'Delete project',exact:true}).click();
 const activeDeletion=page.getByRole('dialog',{name:'Delete “New dog story”?',exact:true});
 await activeDeletion.getByRole('button',{name:'Cancel'}).click();
 assert(projects.some(p=>p.id==='new'&&p.is_active));
 await page.getByRole('button',{name:'Delete project',exact:true}).click();
 await activeDeletion.getByRole('button',{name:'Delete project',exact:true}).click();
 await page.waitForURL('**/projects');
 assert(!projects.some(p=>p.id==='new'));assert(projects.some(p=>p.is_active));
 assert.deepEqual(errors,[]);report.push({width,checks:['folder/table/search/sort','view persistence','detail isolation','explicit style activation','creation error preservation','dirty cancel','activation retry without duplication','rename','export','import','delete','list error retry','zero axe violations','no page overflow','no browser errors'],writes});console.log('PASS project browser',width);await ctx.close();
}}finally{await writeFile(out+'/report.json',JSON.stringify(report,null,2));await browser.close()}
