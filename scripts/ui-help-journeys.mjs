// Public help UI against the production server; controlled failures are browser-local.
import assert from 'node:assert/strict';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost')throw Error('Use a disposable localhost server.');
const output=resolve(process.env.AJENT_HELP_OUTPUT||'artifacts/help-journeys');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||undefined});
const page=await browser.newPage({viewport:{width:390,height:844},colorScheme:'dark',reducedMotion:'reduce'});
page.setDefaultTimeout(10000);
const checks=[],failures=[],errors=[];
page.on('pageerror',error=>errors.push(error.message));
async function check(name,run){try{await page.goto(origin+'/guide');await run();checks.push(name);console.log('PASS '+name);}catch(error){failures.push({name,error:error.message});console.log('FAIL '+name+': '+error.message);}finally{await page.unroute('**/api/chat');}}
async function open(){await page.locator('#chat-launcher').focus();await page.keyboard.press('Enter');await page.locator('dialog[open]').waitFor();}
const input=page.locator('#chat-question');
try{
 await check('chat opens by keyboard and returns focus after Escape',async()=>{
  await open();assert.equal(await page.evaluate(()=>document.activeElement.id),'chat-question');
  for(let i=0;i<12;i++){await page.keyboard.press('Tab');assert.equal(await page.evaluate(()=>document.activeElement===document.body||!!document.activeElement.closest('#ajent-chat')),true);}
  await page.keyboard.press('Escape');await page.waitForFunction(()=>!document.querySelector('#ajent-chat').open&&document.activeElement.id==='chat-launcher');assert.equal(await page.evaluate(()=>document.activeElement.id),'chat-launcher');
 });
 await check('failed answer and retry preserve the next question being typed',async()=>{
  await open();let release,requested;
  const waiting=new Promise(r=>release=r),request=new Promise(r=>requested=r);
  await page.route('**/api/chat',async route=>{requested();await waiting;await route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:'Synthetic service interruption.'})});});
  await input.fill('First question');await input.press('Enter');await request;
  await input.fill('My next unfinished question');release();
  await page.getByText('Synthetic service interruption.',{exact:false}).waitFor();
  await page.waitForFunction(()=>!document.getElementById('chat-send').disabled);
  assert.equal(await input.inputValue(),'My next unfinished question');
  await page.unroute('**/api/chat');let retried;
  await page.route('**/api/chat',route=>{retried=route.request().postDataJSON().messages.at(-1).content;return route.fulfill({contentType:'application/json',body:JSON.stringify({answer:'The retry worked.',mode:'guide',citations:[]})});});
  await page.getByRole('button',{name:'Try this question again',exact:true}).click();
  await page.getByText('The retry worked.',{exact:false}).waitFor();
  assert.equal(retried,'First question');assert.equal(await input.inputValue(),'My next unfinished question');
 });
 await check('suggested questions preserve an unfinished custom question',async()=>{
  await open();await page.route('**/api/chat',route=>route.fulfill({contentType:'application/json',body:JSON.stringify({answer:'Synthetic guide answer.',mode:'guide',citations:[]})}));
  await input.fill('My custom question');await page.getByRole('button',{name:'Why join?',exact:true}).click();
  await page.getByText('Synthetic guide answer.',{exact:false}).waitFor();
  assert.equal(await input.inputValue(),'My custom question');
 });
 await check('multiline questions grow to expose multiple lines',async()=>{
  await open();const before=await input.evaluate(el=>el.clientHeight);
  await input.fill('First line\nSecond line\nThird line');
  const size=await input.evaluate(el=>({height:el.clientHeight,line:parseFloat(getComputedStyle(el).lineHeight)}));
  assert.ok(size.height>before&&size.height>=size.line*3,JSON.stringify({before,...size}));
 });
 await check('canceling New chat preserves the unfinished question',async()=>{
  await open();await input.fill('Keep this unfinished question');let confirmed=false;
  page.once('dialog',async dialog=>{confirmed=true;await dialog.dismiss();});
  await page.getByRole('button',{name:'New chat',exact:true}).click();
  assert.equal(await input.inputValue(),'Keep this unfinished question');assert.equal(confirmed,true);
 });
 await check('answer completion preserves the control the user focused',async()=>{
  await open();let release,requested;const waiting=new Promise(r=>release=r),request=new Promise(r=>requested=r);
  await page.route('**/api/chat',async route=>{requested();await waiting;await route.fulfill({contentType:'application/json',body:JSON.stringify({answer:'Synthetic completed answer.',mode:'guide',citations:[]})});});
  await input.fill('A question');await input.press('Enter');await request;
  await page.locator('#chat-close').focus();release();await page.locator('.chat-mode').waitFor();
  assert.equal(await page.evaluate(()=>document.activeElement.id),'chat-close');
 });
 await check('confirming New chat clears the question and conversation',async()=>{
  await open();await input.fill('Discard this question');page.once('dialog',dialog=>dialog.accept());
  await page.getByRole('button',{name:'New chat',exact:true}).click();
  assert.equal(await input.inputValue(),'');await page.getByText('A fresh start. What would you like to know about Ajent?',{exact:true}).waitFor();
 });
 await check('published guide answer has a usable source link',async()=>{
  await open();await input.fill('What stays private when I connect?');await input.press('Enter');
  await page.locator('.chat-mode').waitFor();const source=page.locator('.chat-sources a').first();await source.waitFor();
  assert.match(await source.getAttribute('href'),/^\/guide#[a-z0-9-]+$/);
  assert.equal(await page.locator('.chat-message.ai').last().textContent().then(t=>t.length>50),true);
 });
 await check('sign-in recovery has a keyboard shortcut and usable repair links',async()=>{
  await page.goto(origin+'/connect-login');await page.locator('#login-recovery').waitFor();
  assert.equal(await page.getByRole('link',{name:'Skip to content',exact:true}).count(),1);
  await page.keyboard.press('Tab');assert.equal(await page.evaluate(()=>document.activeElement.textContent),'Skip to content');
  await page.keyboard.press('Enter');assert.equal(new URL(page.url()).hash,'#main');
  assert.equal(await page.getByRole('link',{name:'Sign in with a saved access key',exact:true}).getAttribute('href'),'/login');
  assert.equal(errors.length,0,errors.join('\n'));
 });
 await page.screenshot({path:output+'/help-mobile-dark.png'});
}finally{
 await writeFile(output+'/results.json',JSON.stringify({suite:'help journeys',executed:checks.length+failures.length,passed:checks.length,failed:failures.length,checks,failures},null,2));
 await browser.close();
}
console.log(JSON.stringify({suite:'help journeys',executed:checks.length+failures.length,failed:failures.length}));
if(failures.length)process.exitCode=1;
