// Isolated real-Go fixture + fake native CLI/providers. Never point at live data.
import {mkdir,writeFile} from 'node:fs/promises';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:5195',api=process.env.E2E_API_URL||'http://127.0.0.1:18091',out=process.env.E2E_OUTPUT||'/tmp/videoflow-story-characters-e2e';await mkdir(out,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']}),report={checks:[],errors:[]};
try{for(const width of [1440,390]){
 const context=await browser.newContext({viewport:{width,height:900}});await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));const page=await context.newPage();page.setDefaultTimeout(20000);page.on('pageerror',e=>report.errors.push(e.message));
 const read=async path=>{const r=await page.request.get(api+path);assert(r.ok(),path);return r.json()};
 const post=async(path,data)=>{const r=await page.request.post(api+path,{data});assert(r.ok(),path+': '+await r.text());return r.json()};
 const check=async(name,fn)=>{await fn();report.checks.push({name:width+': '+name,status:'pass'});console.log('PASS',width,name)};
 await read('/e2e/counters'); // Refuse production mutation.
 const project=await post('/projects',{name:'Character workflow '+width});await post('/projects/'+project.id+'/activate',{});
 const session=await post('/director/sessions',{engine:'codex',title:'Puppy story'});
 const stale=await post('/characters',{name:'Deleted reference'});
 const selection=await page.request.put(api+'/director/sessions/'+session.id+'/selection',{data:{character_ids:[stale.id],asset_ids:[]}});assert(selection.ok());
 assert((await page.request.delete(api+'/characters/'+stale.id)).ok());
 const before=await read('/e2e/counters');let submissions=[];
 page.on('request',r=>{if(new URL(r.url()).pathname==='/videos/generate')submissions.push(r.postDataJSON())});
 await check('Empty project shows explicit profile option; opt-out creates no characters',async()=>{
  await page.goto(base+'/create');const option=page.getByRole('checkbox',{name:'Create missing characters from the story',exact:true});await option.waitFor();assert(await option.isChecked());await option.uncheck();
  await page.getByLabel('What video do you want to make?',{exact:true}).fill('A lost puppy finds his mother');await page.getByRole('combobox',{name:'Format',exact:true}).selectOption('16:9');await page.getByText('More options',{exact:true}).click();await page.getByRole('spinbutton',{name:/Target duration in seconds/}).fill('1');await page.getByRole('button',{name:'Create video',exact:true}).click();
  await page.getByText('Your keyframes are ready for review.',{exact:true}).waitFor({timeout:45000});assert.equal((await read('/characters')).length,0);assert.equal(submissions[0].create_characters,false);assert.equal((await read('/e2e/counters')).videos,before.videos);
 });
 await check('Needed profiles, cast and dialogue persist despite stale session references',async()=>{
  await page.getByRole('button',{name:'Create another story',exact:true}).click();await page.getByRole('checkbox',{name:'Create missing characters from the story',exact:true}).check();await page.getByRole('button',{name:'Create video',exact:true}).click();await page.getByRole('link',{name:'Review characters',exact:true}).waitFor({timeout:45000});
  const characters=await read('/characters');assert.deepEqual(characters.map(c=>c.name).sort(),['Mother dog','Pip']);assert.equal(submissions[1].create_characters,true);assert.deepEqual(submissions[1].character_ids,[]);
  const scenes=await read('/scenes');const scene=scenes.find(s=>s.scene_json?.spoken_lines?.length);assert(scene);assert.equal(scene.character_ids_json.length,2);assert(characters.some(c=>c.id===scene.scene_json.spoken_lines[0].character_id));assert(scene.scene_json.generation_brief.character_notes.includes('small golden puppy'));
  assert((await read('/director/sessions/'+session.id+'/selection')).character_ids.includes(stale.id),'Saved session selection was silently rewritten');
 });
 await check('Character sheets generate automatically and block storyboarding until approved',async()=>{
  await page.getByRole('heading',{name:'Your character references are ready',exact:true}).waitFor();assert(await page.getByRole('button',{name:'Continue storyboarding',exact:true}).isDisabled());
  const characters=await read('/characters');const assets=await read('/assets');assert.equal((await read('/e2e/counters')).images,before.images+3);
  for(const character of characters){assert.equal(character.reference_asset_ids_json.length,1);const asset=assets.find(a=>a.id===character.reference_asset_ids_json[0]);assert.equal(asset.character_id,character.id);assert.equal(asset.type,'character_reference');assert.equal(asset.metadata_json.consistency_review,'pending');const response=await page.request.post(api+'/assets/'+asset.id+'/review',{data:{approved:true,expected_updated_at:asset.updated_at}});assert(response.ok(),await response.text())}
  await page.getByRole('button',{name:'Refresh character approvals',exact:true}).click();await page.waitForFunction(()=>Array.from(document.querySelectorAll('button')).some(b=>b.textContent.trim()==='Continue storyboarding'&&!b.disabled));await page.getByRole('button',{name:'Continue storyboarding',exact:true}).click();await page.getByText('Your keyframes are ready for review.',{exact:true}).waitFor({timeout:45000});
  assert.equal((await read('/e2e/counters')).images,before.images+4);assert.equal((await read('/characters')).length,2);
 });
 await check('Scene review and rendering remain gated with no automatic videos',async()=>{
  assert(await page.getByRole('button',{name:'Render scene 1',exact:true}).isDisabled());const counters=await read('/e2e/counters');assert.equal(counters.videos,before.videos);assert.equal(counters.images,before.images+4);assert.equal(await page.getByRole('link',{name:'Review characters',exact:true}).getAttribute('href'),'/library?tab=characters');assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1),false);await page.screenshot({path:out+'/review-'+width+'.png',fullPage:true});
 });
 await context.close();
}}catch(e){report.failure=e.stack;process.exitCode=1;console.error(e)}finally{await browser.close();await writeFile(out+'/report.json',JSON.stringify(report,null,2));}if(report.errors.length)process.exitCode=1;
