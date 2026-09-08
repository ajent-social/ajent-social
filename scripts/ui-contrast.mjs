// Run after AJENT_VISUAL_OUTPUT=/absolute/path go test ./internal/web -run TestVisualFixtures.
// Every checked page is rendered from the shipped templates, with synthetic data.
import { chromium, firefox, webkit } from 'playwright';
import { createServer } from 'node:http';
import { readFile, readdir, mkdir, writeFile } from 'node:fs/promises';
import { resolve, extname } from 'node:path';
import { createRequire } from 'node:module';
const require=createRequire(import.meta.url);
const root=resolve(process.env.AJENT_VISUAL_OUTPUT||'artifacts/ui');
const files=(await readdir(root)).filter(f=>f.endsWith('.html')).sort();
const expectedFiles=['workspaces','workspaces-empty','workspaces-long','human-login','human-account','human-recovery','human-consent','domains','explore','verification','participation','join','landing','login','feed','settings','agents','groups','notifications','reliability','feed-populated','profile-populated','thread-populated','agents-populated','groups-populated','settings-populated','public-error','verification-populated'].map(name=>name+'.html').sort();
if(JSON.stringify(files)!==JSON.stringify(expectedFiles))throw Error(`Rendered fixture set is incomplete or stale: expected ${expectedFiles.length} named pages, found ${files.join(', ')}`);
const axe=await readFile(require.resolve('axe-core/axe.min.js'),'utf8');
const server=createServer(async(req,res)=>{
 const path=resolve(root,'.'+new URL(req.url,'http://localhost').pathname);
 if(!path.startsWith(root+'/')){res.writeHead(403).end();return}
 try{const b=await readFile(path);res.setHeader('Cache-Control','no-store');res.setHeader('Content-Type',({'.html':'text/html','.css':'text/css','.js':'application/javascript','.png':'image/png','.ttf':'font/ttf'})[extname(path)]||'application/octet-stream');res.end(b)}catch{res.writeHead(404).end()}
});
await new Promise(r=>server.listen(0,'127.0.0.1',r));
const origin=`http://127.0.0.1:${server.address().port}`;
const engines={chromium,firefox,webkit};
const browserNames=(process.env.AJENT_UI_BROWSERS||'chromium').split(',').map(s=>s.trim());
if(!browserNames.length||new Set(browserNames).size!==browserNames.length||browserNames.some(name=>!engines[name]))throw Error('Choose unique browser names from chromium, firefox, webkit.');
const results=[];
await mkdir(root+'/screenshots',{recursive:true});
try{
 for(const browserName of browserNames){
 const browser=await engines[browserName].launch({executablePath:browserName==='chromium'?(process.env.CHROME_PATH||undefined):undefined});
 try{
 for(const colorScheme of ['light','dark'])for(const width of [320,390,1024,1440,2880]){
  const context=await browser.newContext({colorScheme,viewport:{width,height:1000},reducedMotion:'reduce'});
  // Rendering audits do not call live services, enroll accounts, or submit forms.
  await context.route('**/*',route=>route.request().url().startsWith(origin)?route.continue():route.abort());
  const page=await context.newPage();
  for(const file of files){
   const response=await page.goto(origin+'/'+file);
   if(response.status()!==200)throw Error(`${file}: HTTP ${response.status()}`);
   await page.evaluate(()=>document.fonts.ready);
   if(file==='human-recovery.html')await page.evaluate(()=>{document.querySelector('#human-signin').hidden=true;document.querySelector('#human-recovery-result').hidden=false;document.querySelector('#recovery-code').textContent='synthetic-recovery-code'});
   if(file==='human-consent.html')await page.evaluate(()=>{document.querySelector('#installation-consent').hidden=false;document.querySelector('#installation-description').textContent='Approve installation: research';document.querySelector('#installation-code').textContent='123456'});
   if(file==='verification-populated.html')await page.locator('details').evaluate(el=>el.open=true);
   await page.addScriptTag({content:axe});
   const audit=await page.evaluate(async()=>{
    const a=await axe.run(document,{runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21a','wcag21aa']}});
    return {violations:a.violations.map(v=>({id:v.id,nodes:v.nodes.map(n=>({target:n.target,summary:n.failureSummary}))})),contrastChecks:a.passes.find(p=>p.id==='color-contrast')?.nodes.length||0,overflow:document.documentElement.scrollWidth>innerWidth};
   });
   if(!audit.contrastChecks)throw Error(`${file}: no executed contrast checks`);
   results.push({browser:browserName,file,colorScheme,width,...audit});
   if(width===390||width===1440)await page.screenshot({path:`${root}/screenshots/${browserName}-${file}-${colorScheme}-${width}.png`,fullPage:!audit.overflow});
  }
  await context.close();
 }
}finally{await browser.close();}
 }
}finally{await new Promise(r=>server.close(r))}
await writeFile(root+'/results.json',JSON.stringify(results,null,2));
const failures=results.filter(r=>r.violations.length||r.overflow);
console.log(JSON.stringify({checks:results.length,failed:failures.length,failures},null,2));
if(failures.length)process.exitCode=1;
