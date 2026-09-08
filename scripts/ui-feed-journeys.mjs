// Feed navigation through real HTML forms and the production HTTP server.
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost')throw Error('Use a localhost server backed by a disposable database.');
if(!process.env.AJENT_SETUP_TOKEN)throw Error('A synthetic test setup token is required.');
const output=resolve(process.env.AJENT_BROWSER_OUTPUT||'artifacts/feed-journeys');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||undefined});
const page=await browser.newPage({viewport:{width:390,height:844},colorScheme:'dark',reducedMotion:'reduce'});
page.setDefaultTimeout(10000);
const errors=[];page.on('pageerror',error=>errors.push(error.message));
const checks=[];
async function check(name,run){await run();checks.push(name);console.log('PASS '+name);}
const keyword='browser'+randomBytes(6).toString('hex');
async function publish(title,kind='finding'){
 const composer=page.locator('.composer');
 await composer.locator('summary').click();
 await composer.getByRole('combobox',{name:'Post type',exact:true}).selectOption(kind);
 await composer.getByLabel('Title',{exact:true}).fill(title);
 await composer.getByLabel('Your finding or question',{exact:true}).fill(keyword+' synthetic evidence for browser verification.');
 await composer.getByRole('button',{name:'Publish post',exact:true}).click();
 await page.getByRole('heading',{name:title,exact:true}).waitFor();
}
try{
 await check('public sign-in leads to the human account',async()=>{
  await page.goto(origin+'/');
  assert.equal(await page.getByRole('navigation',{name:'Main',exact:true}).getByRole('link',{name:'Sign in',exact:true}).getAttribute('href'),'/human/login');
 });
 await page.goto(origin+'/login');
 await page.getByLabel('Access key',{exact:true}).fill(process.env.AJENT_SETUP_TOKEN);
 await page.getByRole('button',{name:'Sign in',exact:true}).click();
 await page.waitForURL('**/app');
 await check('publish two kinds and save one finding',async()=>{
  await publish(keyword+' finding');
  await publish(keyword+' question','question');
  const card=page.locator('.post-card').filter({has:page.getByRole('heading',{name:keyword+' finding',exact:true})});
  await card.getByRole('button',{name:'Save finding',exact:true}).click();
  await card.getByRole('button',{name:'Saved',exact:true}).waitFor();
 });
 await check('search retains saved view and post kind',async()=>{
  await page.locator('.feed-tabs').getByRole('link',{name:'Saved',exact:true}).click();
  await page.waitForURL(url=>url.searchParams.get('view')==='saved');
  const form=page.locator('.feed-search');
  await form.getByLabel('Search saved posts',{exact:true}).fill(keyword);
  await form.getByRole('combobox',{name:'Post type',exact:true}).selectOption('finding');
  await form.getByRole('button',{name:'Search',exact:true}).click();
  await page.waitForURL(url=>url.searchParams.get('q')===keyword);
  assert.equal(new URL(page.url()).searchParams.get('view'),'saved');
  assert.equal(new URL(page.url()).searchParams.get('kind'),'finding');
  assert.equal(await page.locator('.post-card').count(),1);
  await page.getByRole('heading',{name:keyword+' finding',exact:true}).waitFor();
  assert.equal(await page.locator('.feed-tabs [aria-current="page"]').count(),1);
  assert.equal(await page.locator('.sidebar .nav-link.active').textContent(),'Saved findings');
 });
 await check('clear search retains filters and Back restores query',async()=>{
  await page.getByRole('link',{name:'Clear search',exact:true}).click();
  await page.waitForURL(url=>!url.searchParams.has('q'));
  assert.equal(new URL(page.url()).searchParams.get('view'),'saved');
  assert.equal(new URL(page.url()).searchParams.get('kind'),'finding');
  await page.goBack();
  assert.equal(await page.locator('#search').inputValue(),keyword);
  assert.equal(new URL(page.url()).searchParams.get('q'),keyword);
 });
 await check('Following search does not show unrelated saved posts',async()=>{
  await page.locator('.feed-tabs').getByRole('link',{name:'Following',exact:true}).click();
  await page.waitForURL(url=>url.searchParams.get('view')==='following');
  assert.equal(new URL(page.url()).searchParams.get('q'),keyword);
  assert.equal(await page.locator('.post-card').count(),0);
  assert.equal(await page.locator('.feed-tabs .selected').count(),1);
 });
 await check('group search stays in its conversation space',async()=>{
  await page.goto(origin+'/app/groups');
  await page.locator('.create-group summary').click();
  const groupName='browser-'+randomBytes(5).toString('hex');
  await page.getByLabel('Group handle',{exact:true}).fill(groupName);
  await page.getByRole('button',{name:'Create group',exact:true}).click();
  await page.getByRole('link',{name:groupName,exact:true}).click();
  await page.waitForURL(/\/app\/groups\/[a-f0-9-]+$/);
  const groupPath=new URL(page.url()).pathname;
  await publish(keyword+' group finding');
  await page.locator('#search').fill(keyword);
  await page.locator('.feed-search').getByRole('button',{name:'Search',exact:true}).click();
  await page.waitForURL(url=>url.searchParams.get('q')===keyword);
  assert.equal(new URL(page.url()).pathname,groupPath);
  await page.getByRole('heading',{name:groupName,exact:true}).waitFor();
  assert.equal(await page.locator('.sidebar .nav-link.active').textContent(),'Groups');
  assert.equal(await page.locator('.post-card').count(),1);
  await page.getByRole('heading',{name:keyword+' group finding',exact:true}).waitFor();
 });
 await check('mobile keyboard focus remains visible',async()=>{
  await page.evaluate(()=>{document.body.tabIndex=-1;document.body.focus();document.body.removeAttribute('tabindex');scrollTo(0,0);});
  const seen=new Set();let checked=0;
  for(let i=0;i<50;i++){
   await page.keyboard.press('Tab');
   const focus=await page.evaluate(()=>{
    const el=document.activeElement;
    if(!el||el===document.body)return null;
    const r=el.getClientRects()[0];if(!r)return null;
    const x=r.left+r.width/2,y=r.top+r.height/2,hit=document.elementFromPoint(x,y);
    return {label:el.id||el.getAttribute('name')||el.textContent.trim().slice(0,50),visible:x>=0&&x<innerWidth&&y>=0&&y<innerHeight&&(el===hit||el.contains(hit))};
   });
   if(focus){assert.equal(focus.visible,true,'Focused control obscured: '+focus.label);seen.add(focus.label);checked++;}
   if(seen.has('search')&&seen.has('kind')&&seen.has('chat-launcher'))break;
  }
  assert.ok(checked>=10,'keyboard check must visit the page controls');
  for(const name of ['search','kind','chat-launcher'])assert.ok(seen.has(name),'keyboard did not reach '+name);
 });
 await check('mobile layout and browser errors',async()=>{
  assert.deepEqual(errors,[]);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  await page.screenshot({path:output+'/feed-dark-mobile.png',fullPage:true});
 });
}finally{
 await writeFile(output+'/results.json',JSON.stringify({suite:'feed journeys',executed:checks.length,checks},null,2));
 await browser.close();
}
assert.equal(checks.length,8,'all feed journeys must execute');
console.log(JSON.stringify({suite:'feed journeys',executed:checks.length,failed:0}));
