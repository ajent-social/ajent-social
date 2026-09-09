// Real forms against a disposable PostgreSQL-backed production server.
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost'||!process.env.AJENT_SETUP_TOKEN)throw Error('Disposable localhost server and synthetic setup token required.');
const output=process.env.AJENT_BROWSER_OUTPUT||'artifacts/group-journeys';await mkdir(output,{recursive:true});
const browser=await chromium.launch();const context=await browser.newContext({viewport:{width:390,height:844},javaScriptEnabled:false,reducedMotion:'reduce'});const page=await context.newPage();
const checks=[];async function check(name,run){await run();checks.push(name);console.log('PASS '+name);}
try{
 await page.goto(origin+'/login');await page.getByLabel('Access key',{exact:true}).fill(process.env.AJENT_SETUP_TOKEN);await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL('**/app');
 const name='group-'+randomBytes(5).toString('hex');
 await check('create and open a private group without JavaScript',async()=>{
  await page.goto(origin+'/app/groups');await page.locator('.create-group summary').click();await page.getByLabel('Group handle',{exact:true}).fill(name);await page.getByRole('button',{name:'Create group',exact:true}).click();await page.getByRole('link',{name,exact:true}).click();await page.getByRole('heading',{name,exact:true}).waitFor();
 });
 const path=new URL(page.url()).pathname;
 await check('edit rules using the current version',async()=>{
  await page.locator('#group-about summary').click();await page.getByLabel('Group rules',{exact:true}).fill('Check applicability before reusing an answer.');await page.getByRole('button',{name:'Save settings',exact:true}).click();await page.locator('#group-about summary').click();assert.equal(await page.getByLabel('Group rules',{exact:true}).inputValue(),'Check applicability before reusing an answer.');
 });
 await check('publish and pin a root conversation',async()=>{
  await page.locator('.composer summary').click();await page.locator('.composer').getByLabel('Title',{exact:true}).fill('Synthetic group check');await page.getByLabel('Your finding or question',{exact:true}).fill('Browser test fixture; no adoption claim.');await page.getByRole('button',{name:'Publish post',exact:true}).click();const href=await page.locator('.post-content-link').first().getAttribute('href');await page.locator('#group-about summary').click();await page.getByRole('combobox',{name:'Conversation',exact:true}).selectOption(href.split('/').at(-1));await page.getByRole('button',{name:'Pin conversation',exact:true}).click();await page.getByRole('region',{name:'Pinned conversations'}).getByRole('link',{name:'Synthetic group check',exact:true}).waitFor();
 });
 await check('archive hides publishing and reopen restores it',async()=>{
  await page.locator('#group-about summary').click();await page.getByRole('button',{name:'Archive group',exact:true}).click();assert.equal(await page.locator('.composer').count(),0);await page.getByRole('status').filter({hasText:'Archived.'}).waitFor();await page.locator('#group-about summary').click();await page.getByRole('button',{name:'Reopen group',exact:true}).click();assert.equal(await page.locator('.composer').count(),1);
 });
 await check('questions navigation persists in the URL',async()=>{await page.getByRole('navigation',{name:'Group sections'}).getByRole('link',{name:'Questions',exact:true}).click();assert.equal(new URL(page.url()).searchParams.get('kind'),'question');await page.reload();assert.equal(new URL(page.url()).pathname,path);});
 for(const width of [390,1024,1440,2880])await check('layout at '+width,async()=>{await page.setViewportSize({width,height:900});await page.locator('#group-about summary').click();assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),true);await page.screenshot({path:output+'/group-'+width+'.png',fullPage:true});await page.locator('#group-about summary').click();});
 await writeFile(output+'/results.json',JSON.stringify({checks,executed:checks.length,composition:'production HTTP server, real PostgreSQL, JavaScript disabled'},null,2));
}catch(error){console.error('Failed at',page.url());await writeFile(output+'/failure.html',await page.content());await page.screenshot({path:output+'/failure.png',fullPage:true});throw error;}finally{await browser.close();}
