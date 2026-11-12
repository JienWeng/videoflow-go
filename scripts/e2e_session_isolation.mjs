import {pathToFileURL} from 'node:url';
const {chromium}=await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE||'/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const browser=await chromium.launch({executablePath:process.env.CHROME_BIN||'/usr/bin/google-chrome',headless:true,args:['--no-sandbox']});
try{
 const c=await browser.newContext({viewport:{width:1440,height:1000}});await c.addInitScript(()=>localStorage.setItem('videoflow.onboarded','1'));let selected='a',pendingChat,slowA=false,pendingHistory;
 await c.route('**/api/**',async r=>{const p=new URL(r.request().url()).pathname.replace('/api','');let body={};
 if(p==='/director/status')body={all_text_local:true,engines:[{engine:'codex',installed:true}]};
 else if(p==='/director/sessions')body=['a','b'].map(id=>({id,title:'Session '+id,engine:'codex',status:'ready',selected:id===selected}));
 else if(p.endsWith('/select'))selected=p.split('/')[3];
 else if(p.endsWith('/turns')){if(p.includes('/a/')&&slowA){pendingHistory=r;return;}body=[{kind:'chat',request_json:{user_message:'History '+p.split('/')[3]},result_json:{reply:'Saved response '+p.split('/')[3]}}];}
 else if(p==='/chat'){pendingChat=r;return;}
 else if(p==='/graph')body={nodes:[],edges:[]};else if(p==='/projects/active')body={id:'p',name:'Session QA'};
 else if(['/characters','/assets','/scenes','/ops'].includes(p))body=[];
 return r.fulfill({json:body});});
 const page=await c.newPage();await page.goto((process.env.E2E_BASE_URL||'http://127.0.0.1:8080')+'/?view=studio');await page.getByText('History a',{exact:true}).waitFor();const composer=page.locator('textarea[placeholder="Describe your story or select an item…"]');await composer.fill('Request in A');await composer.press('Enter');for(let i=0;!pendingChat&&i<1000;i++)await new Promise(r=>setTimeout(r,10));if(!pendingChat)throw Error('Chat was not submitted');
 await page.getByRole('button',{name:'AI session',exact:true}).click();await page.getByRole('menuitemradio',{name:/Session b/}).click();await page.getByText('History b',{exact:true}).waitFor();await pendingChat.fulfill({json:{intent:{action:'unknown',reply:'LATE A RESPONSE'},options:{},state:{}}});await page.waitForTimeout(150);if(await page.getByText('LATE A RESPONSE',{exact:true}).count())throw Error('A response leaked into B');console.log('PASS delayed A response excluded from B');
 slowA=true;await page.getByRole('button',{name:'AI session',exact:true}).click();await page.getByRole('menuitemradio',{name:/Session a/}).click();for(let i=0;!pendingHistory&&i<1000;i++)await new Promise(r=>setTimeout(r,10));if(!pendingHistory)throw Error('History was not requested');await page.getByRole('button',{name:'AI session',exact:true}).click();await page.getByRole('menuitemradio',{name:/Session b/}).click();await page.getByText('History b',{exact:true}).waitFor();await pendingHistory.fulfill({json:[{kind:'chat',request_json:{user_message:'STALE A HISTORY'},result_json:{reply:'stale'}}]});await page.waitForTimeout(150);if(await page.getByText('STALE A HISTORY',{exact:true}).count())throw Error('Stale history replaced B');console.log('PASS delayed history restore excluded');await c.close();
}finally{await browser.close();}
