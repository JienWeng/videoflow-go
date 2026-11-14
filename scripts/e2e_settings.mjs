// Uses the repository's Playwright fixture workflow; no real keys or generation requests.
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
const base=process.env.E2E_BASE_URL||'http://127.0.0.1:8080';
try {
for (const width of [1440,390]) {
 const context=await browser.newContext({viewport:{width,height:900}});
 const page=await context.newPage(); let failLoad=false,failSave=false; const mutations=[]; const errors=[]; page.on('pageerror',e=>errors.push(e.message));
 let app={all_text_local:true,default_aspect_ratio:'16:9',default_scene_duration:5,dialogue_language:'English',caption_style:'clean',caption_language:'auto',whisper_model:'saved-transcription-model',render_negatives:['blurry'],default_image_provider:'fal',default_video_provider:'fal',image_model:'image',ref_image_model:'image',video_model:'video',vl_model:'vision',media_model_preferences:{}};
 await page.route('**/api/**',async r=>{
  const u=new URL(r.request().url()).pathname,method=r.request().method(); let body={};
  if(method!=='GET')mutations.push({u,body:r.request().postDataJSON()});
  if(u.endsWith('/settings/app')) {if(failLoad||method==='PUT'&&failSave)return r.fulfill({status:503,json:{error:'Fixture temporarily unavailable'}});if(method==='PUT')app={...app,...r.request().postDataJSON()};body=app;}
  else if(u.endsWith('/settings/providers'))body=[{name:'fal',label:'fal.ai',configured:true,protocol:'media',models:['image'],suggested_models:[],default_model:'image'}];
  else if(u.endsWith('/settings/providers/fal'))body={name:'fal',configured:true,from_db:true,masked_key:'••••',base_url:''};
  else if(u.includes('/models'))body={models:[],capabilities:[]};
  else if(u.endsWith('/projects/active'))body={id:'p',name:'Test project'};
  else if(/\/(agents|ops|render-jobs|sessions|providers)$/.test(u))body=[];
  return r.fulfill({json:body});
 });
 await page.goto(base+'/settings'); await page.getByRole('heading',{name:'Appearance',exact:true}).waitFor();
 assert.equal(await page.getByRole('heading',{name:'Render negative prompts'}).count(),0);
 await page.getByRole('radio',{name:'Light',exact:true}).click(); await page.keyboard.press('ArrowRight'); assert.equal(await page.getByRole('radio',{name:'Dark',exact:true}).getAttribute('aria-checked'),'true');
 await page.getByLabel('Default aspect ratio',{exact:true}).selectOption('9:16'); await page.getByRole('status').filter({hasText:'Preferences saved'}).waitFor();
 failSave=true;await page.getByLabel('Default aspect ratio',{exact:true}).selectOption('1:1');await page.getByRole('status').filter({hasText:'Could not save'}).waitFor();assert.equal(await page.getByLabel('Default aspect ratio',{exact:true}).inputValue(),'9:16');failSave=false;
 assert.equal(await page.getByLabel('OpenRouter transcription model').inputValue(),'saved-transcription-model');
 assert.equal(await page.getByLabel('Dialogue language',{exact:true}).inputValue(),'English');
 await page.getByRole('button',{name:'AI connections',exact:true}).click();assert.equal(await page.getByRole('heading',{name:'Media models',exact:true}).count(),0);
 const before=mutations.length;await page.getByRole('button',{name:'Clear key',exact:true}).click();await page.getByRole('group',{name:'Remove fal.ai key'}).waitFor();assert.equal(mutations.length,before);await page.getByRole('button',{name:'Keep key',exact:true}).click();assert.equal(mutations.length,before);
 await page.getByRole('button',{name:'Media models',exact:true}).click(); await page.getByRole('heading',{name:'Media models',exact:true}).waitFor(); assert.equal(new URL(page.url()).hash,'#models');
 await page.getByRole('button',{name:'Advanced',exact:true}).click();await page.getByRole('heading',{name:'Render negative prompts',exact:true}).waitFor();
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.goto(base+'/settings#engines');await page.getByRole('heading',{name:'Media models',exact:true}).waitFor();
 failLoad=true;await page.reload();await page.getByRole('button',{name:'Try again',exact:true}).waitFor();assert.equal(await page.getByRole('button',{name:'General',exact:true}).count(),0);failLoad=false;await page.getByRole('button',{name:'Try again',exact:true}).click();await page.getByRole('heading',{name:'Media models',exact:true}).waitFor();
 assert.deepEqual(errors,[]);console.log(`PASS Settings ${width}: grouping, theme keyboard, save/failure, saved model, key confirmation, deep links, load retry, overflow`);await context.close();
}
} finally {await browser.close();}
