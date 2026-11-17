// Fully mocked style-reference workflow; no live analysis or generation.
import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
import {writeFile} from 'node:fs/promises';
const {chromium}=await import(pathToFileURL('/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const {default:AxeBuilder}=await import(pathToFileURL('/tmp/videoflow-risk-tools/node_modules/@axe-core/playwright/dist/index.mjs'));
const browser=await chromium.launch({executablePath:'/home/ubuntu/.cache/ms-playwright/chromium-1254/chrome-linux-arm64/chrome',headless:true,args:['--no-sandbox']});
const report={checks:[],errors:[]};
try{for(const width of [1440,390]){
 const context=await browser.newContext({viewport:{width,height:1000}}),page=await context.newPage();page.setDefaultTimeout(15000);page.on('pageerror',e=>report.errors.push(e.message));await context.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));
 let style={id:'style',project_id:'qa',name:'Original',style_prompt:'Original illustration',palette:'Blue',lighting:'Soft',tone:'Calm',audience:'Families'},analyses=[],saves=0,chats=0;
 const extracted={name:'Watercolor',style_prompt:'Soft watercolor and simple forms',palette:'Warm ochre',lighting:'Diffuse',tone:'Hopeful',audience:'Families'};
 await context.route('**/api/**',async route=>{const req=route.request(),p=new URL(req.url()).pathname.replace('/api','');let body={};
 if(p==='/events')return route.fulfill({contentType:'text/event-stream',body:': ok\n\n'});
 if(p==='/projects/active')body={id:'qa',name:'Reference project'};else if(p==='/director/sessions')body=[{id:'qa',selected:true,title:'Story planning',engine:'codex',status:'ready'}];else if(p==='/director/status')body={all_text_local:true,engines:[]};else if(p==='/chat/capabilities')body={local:true,subagents:{supported:false}};else if(p==='/graph')body={nodes:[],edges:[]};
 else if(p==='/assets')body=[{id:'video',name:'Look clip',file_path:'storage/look.mp4'}];
 else if(p==='/style'&&req.method()==='PATCH'){style={...style,...req.postDataJSON()};saves++;body=style;}
 else if(p==='/style')body=style;
 else if(p==='/style-presets')body={project_style:{id:'project-style',name:style.name,description:style.style_prompt,fingerprint:'a'.repeat(64)},presets:[{id:'2d-picture-book',name:'Picture book',description:'Soft illustrated shapes',fingerprint:'b'.repeat(64)}]};
 else if(p==='/assets/upload')body={id:'image',name:'Uploaded image',file_path:'storage/look.png'};
 else if(p==='/style/reference'){analyses.push(req.postDataJSON());body={op_id:'reference-op',status:'running'};}
 else if(p==='/ops/reference-op')body={id:'reference-op',status:'succeeded',result_json:{style:extracted,source:analyses.at(-1)?.url||'Look clip',source_kind:analyses.at(-1)?.asset_id==='video'?'sampled_video':'image',sample_count:analyses.at(-1)?.asset_id==='video'?4:1,saved:false}};
 else if(p==='/chat'){chats++;body={intent:{action:'generate_script',reply:'Choose your story look',idea:'Pip finds Mama',creation_brief:{idea:'Pip finds Mama',...req.postDataJSON().creation_brief}},options:{characters:[],scenes:[],outputs:[]},state:{}};}
 else if(['/projects','/characters','/scenes','/ops','/director/sessions/qa/turns'].includes(p))body=[];
 await route.fulfill({json:body});});
 await page.goto('http://127.0.0.1:8091/?view=studio');if(width===390)await page.getByRole('button',{name:/Workbench/}).click();
 const composer=page.getByRole('textbox',{name:'Message story assistant',exact:true});
 await composer.fill('Use https://example.com/look.png as style reference');await composer.press('Enter');
 const dialog=page.getByRole('dialog');await dialog.getByRole('heading',{name:'Style reference',exact:true}).waitFor();assert.equal(await dialog.getByLabel('Reference link',{exact:true}).inputValue(),'https://example.com/look.png');assert.equal(chats,0);
 await dialog.getByRole('button',{name:'Analyze reference',exact:true}).click();await dialog.getByLabel('Visual direction',{exact:true}).waitFor();assert.equal(saves,0);assert.equal(style.name,'Original');
 await dialog.getByLabel('Visual direction',{exact:true}).fill('Watercolor with warm paper texture');
 const axe=await new AxeBuilder({page}).withTags(['wcag2a','wcag2aa','wcag21aa']).analyze();assert.deepEqual(axe.violations,[]);await page.screenshot({path:'/tmp/style-reference-'+width+'.png',fullPage:true});
 await dialog.getByRole('button',{name:'Save as project style',exact:true}).click();await dialog.waitFor({state:'hidden'});assert.equal(saves,1);assert.equal(style.style_prompt,'Watercolor with warm paper texture');
 await composer.fill('Plan a story about Pip finding Mama');await composer.press('Enter');await page.getByRole('button',{name:'Create story & references',exact:true}).waitFor();
 await page.getByRole('button',{name:'Attach style reference',exact:true}).click();await dialog.getByRole('combobox',{name:'Library reference',exact:true}).selectOption('video');await dialog.getByRole('button',{name:'Analyze reference',exact:true}).click();await dialog.getByText(/4 sampled frames/).waitFor();await dialog.getByText('Samples describe the look. Audio, exact pacing and the complete video were not analyzed.',{exact:true}).waitFor();await dialog.getByRole('button',{name:'Save as project style',exact:true}).click();await dialog.waitFor({state:'hidden'});
 assert.equal(await page.getByRole('button',{name:'Create story & references',exact:true}).count(),1);await page.getByText('Earlier creative brief · updated below',{exact:true}).waitFor();await page.getByRole('button',{name:'4. Confirm',exact:true}).click();assert(await page.getByRole('button',{name:'Create story & references',exact:true}).isDisabled());await page.getByText('Project style · Watercolor',{exact:true}).waitFor();
 await page.getByRole('button',{name:'Attach style reference',exact:true}).click();await dialog.getByLabel('Image or video file',{exact:true}).setInputFiles({name:'look.png',mimeType:'image/png',buffer:Buffer.from('fixture')});await dialog.getByText('Reference uploaded to your Library. Analyze it to extract visual direction.',{exact:true}).waitFor();await dialog.getByRole('button',{name:'Analyze reference',exact:true}).click();await dialog.getByLabel('Visual direction',{exact:true}).waitFor();assert.equal(analyses.at(-1).asset_id,'image');
 report.checks.push({width,pass:['chat link opens reference input without text inference','analysis is draft only','editable style saved explicitly','zero axe violations','sampled video limitations shown','active brief uses saved style without duplicate card','fresh confirmation required','image attachment uploaded and analyzed']});console.log('PASS references',width);await context.close();
}}finally{await writeFile('/tmp/style-reference-browser-results.json',JSON.stringify(report,null,2));await browser.close();}
assert.deepEqual(report.errors,[]);
