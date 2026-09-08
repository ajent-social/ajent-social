// Pointer/focus checks against real HTML; help must occupy space, not cover forms.
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {chromium,firefox,webkit} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost')throw Error('Use localhost and a disposable database.');
const output=resolve(process.env.AJENT_BROWSER_OUTPUT||'artifacts/pointer-journeys');await mkdir(output,{recursive:true});
const checks=[];
try{
 for(const [name,engine] of Object.entries({chromium,webkit,firefox})){
  const browser=await engine.launch({executablePath:name==='chromium'?process.env.CHROME_PATH||undefined:undefined});
  try{for(const width of [320,390,1024]){
   const page=await browser.newPage({viewport:{width,height:844},colorScheme:'dark',reducedMotion:'reduce'});page.setDefaultTimeout(10000);
   let chatRequests=0;const errors=[];page.on('pageerror',e=>errors.push(e.message));page.on('request',r=>{if(new URL(r.url()).pathname==='/api/chat')chatRequests++;});
   try{
    await page.goto(origin+'/login');await page.getByLabel('Access key',{exact:true}).fill('synthetic-invalid-pointer-test-key');
    const geometry=await page.evaluate(()=>{
     const dock=document.querySelector('.chat-dock').getBoundingClientRect(),main=document.querySelector('main').getBoundingClientRect();
     const button=document.querySelector('.login-card button'),r=button.getBoundingClientRect();
     const inViewport=r.y+r.height/2>0&&r.y+r.height/2<innerHeight;
     return{dockTop:dock.top,mainBottom:main.bottom,inViewport,buttonReceivesPointer:!inViewport||button.contains(document.elementFromPoint(r.x+r.width/2,r.y+r.height/2))};
    });
    assert.ok(geometry.dockTop>=geometry.mainBottom,'help row must not overlap main content: '+JSON.stringify(geometry));
    assert.equal(geometry.buttonReceivesPointer,true,'visible sign-in button must receive pointer input');
    await page.getByRole('button',{name:'Sign in',exact:true}).click();
    await page.locator('.login-card .error').waitFor({state:'visible'});
    assert.equal(await page.getByLabel('Access key',{exact:true}).getAttribute('aria-invalid'),'true');
    assert.equal(await page.locator('#login-error').getAttribute('role'),'alert');
    const colors=await page.locator('.login-card .error').evaluate(el=>{const style=getComputedStyle(el);return [style.color,style.backgroundColor];});
    const luminance=color=>color.match(/[\d.]+/g).slice(0,3).map(v=>{const n=Number(v)/255;return n<=.04045?n/12.92:((n+.055)/1.055)**2.4;}).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i],0);
    const levels=colors.map(luminance).sort((a,b)=>a-b),contrast=(levels[1]+.05)/(levels[0]+.05);
    assert.ok(contrast>=4.5,'login error text contrast must be at least 4.5:1; got '+contrast.toFixed(2));
    await page.locator('#chat-launcher').click();await page.locator('#ajent-chat').waitFor({state:'visible'});
    await page.locator('#chat-close').click();
    await page.waitForFunction(()=>!document.querySelector('#ajent-chat').open&&document.activeElement===document.querySelector('#chat-launcher'));
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
    assert.deepEqual(errors,[]);assert.equal(chatRequests,0);
    const check=name+' '+width+': pointer sign-in and help focus';checks.push(check);console.log('PASS '+check);
    if(name==='chromium'&&width===390)await page.screenshot({path:output+'/help-footer-mobile.png',fullPage:true});
   }finally{await page.close();}
  }}finally{await browser.close();}
 }
}finally{await writeFile(output+'/results.json',JSON.stringify({suite:'pointer journeys',executed:checks.length,checks},null,2));}
assert.equal(checks.length,9,'all pointer journeys must execute');console.log(JSON.stringify({suite:'pointer journeys',executed:checks.length,failed:0}));
