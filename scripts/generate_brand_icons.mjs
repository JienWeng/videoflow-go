// Deterministically rasterize the canonical SVG; no image-model/API calls.
// Needs Playwright + Chromium. Set PLAYWRIGHT_MODULE / CHROME_BIN for a custom installation.
import{readFile,writeFile,mkdir}from'node:fs/promises';import{fileURLToPath,pathToFileURL}from'node:url';import path from'node:path';
const root=fileURLToPath(new URL('../',import.meta.url)),brand=path.join(root,'frontend/static/brand');
const source=await readFile(path.join(brand,'videoflow-symbol.svg'),'utf8');
const icon=source.replace('  <title>','  <rect width="64" height="64" rx="14" fill="#111615"/>\n  <title>');
await writeFile(path.join(brand,'videoflow-mark.svg'),icon);await writeFile(path.join(brand,'videoflow-wordmark.svg'),source);await writeFile(path.join(brand,'videoflow-symbol-light.svg'),source.replace('#F5EFE7','#19201F'));
const module=process.env.PLAYWRIGHT_MODULE||'playwright';const{chromium}=await import(module.startsWith('/')?pathToFileURL(module).href:module);
const browser=await chromium.launch({...(process.env.CHROME_BIN?{executablePath:process.env.CHROME_BIN}:{}),headless:true,args:['--no-sandbox']});const images=new Map();
try{const page=await browser.newPage();for(const size of[16,32,48,128,180,192,256,512,1024]){await page.setViewportSize({width:size,height:size});await page.setContent('<style>html,body{margin:0;width:100%;height:100%;background:transparent}svg{display:block;width:100%;height:100%}</style>'+icon);images.set(size,await page.screenshot({omitBackground:true}));}
for(const[size,name]of[[16,'favicon-16.png'],[32,'favicon-32.png'],[180,'apple-touch-icon.png'],[192,'icon-192.png'],[512,'icon-512.png']])await writeFile(path.join(brand,name),images.get(size));
// PNG-encoded ICO and modern ICNS containers retain the exact vector geometry.
const sizes=[16,32,48,256],header=Buffer.alloc(6+16*sizes.length);header.writeUInt16LE(1,2);header.writeUInt16LE(sizes.length,4);let offset=header.length;for(let i=0;i<sizes.length;i++){const size=sizes[i],data=images.get(size),at=6+i*16;header[at]=size===256?0:size;header[at+1]=header[at];header.writeUInt16LE(1,at+4);header.writeUInt16LE(32,at+6);header.writeUInt32LE(data.length,at+8);header.writeUInt32LE(offset,at+12);offset+=data.length;}
const records=[];for(const[size,type]of[[128,'ic07'],[256,'ic08'],[512,'ic09'],[1024,'ic10']]){const data=images.get(size),record=Buffer.alloc(8);record.write(type);record.writeUInt32BE(data.length+8,4);records.push(record,data)}const icnsHeader=Buffer.alloc(8);icnsHeader.write('icns');icnsHeader.writeUInt32BE(8+records.reduce((sum,b)=>sum+b.length,0),4);
const desktop=path.join(root,'electron/build');await mkdir(desktop,{recursive:true});await mkdir(path.join(root,'electron/assets'),{recursive:true});await writeFile(path.join(root,'electron/assets/icon.png'),images.get(512));await writeFile(path.join(desktop,'icon.png'),images.get(512));await writeFile(path.join(desktop,'icon.ico'),Buffer.concat([header,...sizes.map(size=>images.get(size))]));await writeFile(path.join(desktop,'icon.icns'),Buffer.concat([icnsHeader,...records]));
// Small-size and theme review sheet; kept outside the product assets.
await page.setViewportSize({width:960,height:500});const sample=(background,foreground)=>'<section style="background:'+background+';color:'+foreground+';padding:32px;display:flex;gap:32px;align-items:center">'+[128,64,32,24,16].map(size=>'<div style="width:'+size+'px;height:'+size+'px;flex:none">'+source.replace('fill="#F5EFE7"','fill="currentColor"')+'</div>').join('')+'</section>';
await page.setContent('<style>body{margin:0;background:#111615;font:14px sans-serif}svg{width:100%;height:100%;display:block}</style>'+sample('#111615','#F5EFE7')+sample('#F7F4EF','#19201F'));
await page.screenshot({path:process.env.BRAND_PREVIEW||'/tmp/videoflow-brand-review.png'});
}finally{await browser.close()}
console.log('Generated web and desktop icons from frontend/static/brand/videoflow-symbol.svg');
