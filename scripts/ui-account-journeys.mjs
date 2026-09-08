// Real browser + WebAuthn + HTTP + PostgreSQL account journeys. Synthetic accounts only.
// Start ajentd against a fresh test database; pass its loopback origin below.
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost')throw Error('AJENT_BROWSER_ORIGIN must use localhost and a disposable database.');
const output=resolve(process.env.AJENT_BROWSER_OUTPUT||'artifacts/account-journeys');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||undefined});
const context=await browser.newContext({viewport:{width:390,height:844},colorScheme:'dark',reducedMotion:'reduce'});
const page=await context.newPage();
page.setDefaultTimeout(10000);
const errors=[];
page.on('pageerror',error=>errors.push(error.message));
const checks=[];
async function check(name,run){await run();checks.push(name);console.log('PASS '+name);}
const cdp=await context.newCDPSession(page);
await cdp.send('WebAuthn.enable');
const addAuthenticator=()=>cdp.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}});
let authenticator=await addAuthenticator();
async function linkInstallation(name){
 const key=`aj_${randomBytes(8).toString('hex')}_${randomBytes(32).toString('base64url')}`;
 const response=await context.request.post(origin+'/v1/installations/link',{headers:{Authorization:'Bearer '+key},data:{handle:'browser-'+randomBytes(5).toString('hex'),project_scope:'project:'+randomBytes(32).toString('hex')}});
 assert.equal(response.status(),200,'installation request must succeed');
 const link=await response.json();
 await page.goto(link.url);
 await page.locator('#installation-consent').waitFor({state:'visible'});
 assert.equal(await page.locator('#installation-code').textContent(),link.code);
 await page.locator('#link-workspace').selectOption('');
 await page.locator('input[name="name"]').fill(name);
 await page.locator('input[name="confirm"]').check();
 await page.getByRole('button',{name:'Approve installation',exact:true}).click();
 await page.waitForURL('**/human/workspaces?**');
 const status=await context.request.get(origin+'/v1/installations/link',{headers:{Authorization:'Bearer '+key}});
 assert.equal(status.status(),200);
 assert.equal((await status.json()).status,'approved');
 return key;
}
let recovery;
try{
 await check('register with Enter and save recovery code',async()=>{
  await page.goto(origin+'/human/login');
  await page.getByLabel('Your name',{exact:true}).fill('Browser Journey');
  await page.getByLabel('Your name',{exact:true}).press('Enter');
  await page.locator('#human-recovery-result').waitFor({state:'visible'});
  recovery=await page.locator('#recovery-code').textContent();
  assert.equal(recovery.length,43);
  assert.equal(await page.locator('#recovery-saved').evaluate(el=>el===document.activeElement),true);
  await page.getByRole('button',{name:'I have saved this code'}).click();
  await page.waitForURL('**/human/workspaces');
  await page.getByRole('heading',{name:'Connect your first installation'}).waitFor();
 });
 await check('explain why the only passkey cannot be removed',async()=>{
  assert.equal(await page.getByRole('button',{name:'Remove passkey',exact:true}).count(),0);
  await page.getByRole('link',{name:'Manage passkeys',exact:true}).click();
  await page.getByText('Your only passkey. Add another before removing it.',{exact:true}).waitFor();
 });
 await check('public pages return a human account to its workspaces',async()=>{
  for(const width of [320,390]){
   await page.setViewportSize({width,height:844});
   for(const path of ['/','/guide','/docs','/domains','/explore','/join','/participation']){
    await page.goto(origin+path);
    const link=page.getByRole('navigation',{name:'Main',exact:true}).getByRole('link',{name:'Your workspaces',exact:true});
    await link.waitFor();assert.equal(await link.getAttribute('href'),'/human/workspaces');
    assert.equal(await page.locator('[data-session-join]').textContent(),'Connect agent');
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,path+' overflow');
   }
  }
  await page.goto(origin+'/human/workspaces');
 });
 await check('approve first installation',async()=>{await linkInstallation('First test workspace');});
 await check('approval opens the newly connected workspace',async()=>{
  await linkInstallation('Second test workspace');
  assert.equal(await page.locator('#workspace-select option:checked').textContent(),'Second test workspace');
  assert.ok(new URL(page.url()).searchParams.get('workspace'));
 });
 await check('agent search, details, refresh and Back preserve the selected view',async()=>{
  await page.getByRole('link',{name:'Agents',exact:true}).click();
  const row=page.locator('.agent-row').first();
  const handle=(await row.locator('.agent-identity strong').textContent()).slice(1);
  await row.locator('summary').click();
  await page.waitForURL(url=>url.hash.startsWith('#agent-'));
  await page.reload();
  assert.equal(await page.locator('.agent-row').first().getAttribute('open'),'');
  await page.getByLabel('Search agents',{exact:true}).fill(handle);
  await page.getByLabel('Search agents',{exact:true}).press('Enter');
  await page.waitForURL(url=>url.searchParams.get('q')===handle);
  assert.equal(await page.locator('.agent-row').count(),1);
  await page.getByLabel('Search agents',{exact:true}).fill('no-matching-agent');
  await page.getByRole('button',{name:'Search',exact:true}).click();
  await page.getByRole('heading',{name:'No matching agents',exact:true}).waitFor();
  await page.goBack();
  assert.equal(await page.getByLabel('Search agents',{exact:true}).inputValue(),handle);
  assert.equal(await page.locator('.agent-row').count(),1);
 });
 await check('revocation keeps the selected workspace',async()=>{
  const workspace=new URL(page.url()).searchParams.get('workspace');
  await page.getByRole('link',{name:'Installations',exact:true}).click();
  await page.getByText('Manage installation',{exact:true}).click();
  page.once('dialog',dialog=>dialog.accept());
  await page.getByRole('button',{name:'Revoke installation and agents',exact:true}).click();
  await page.waitForURL(url=>url.searchParams.get('notice')==='Access revoked');
  assert.equal(new URL(page.url()).searchParams.get('workspace'),workspace);
  assert.equal(new URL(page.url()).searchParams.get('view'),'installations');
  assert.match(await page.locator('.installation-row').first().textContent(),/Revoked/);
 });
 await check('invalid domain keeps the form and focuses its error',async()=>{
  await page.getByRole('link',{name:'Domains',exact:true}).click();
  await page.getByLabel('Domain',{exact:true}).fill('not a domain');
  await page.getByRole('button',{name:'Create DNS proof',exact:true}).click();
  await page.getByRole('status').filter({hasText:'Enter a DNS domain.'}).waitFor();
  assert.equal(await page.getByLabel('Domain',{exact:true}).inputValue(),'not a domain');
  assert.equal(await page.locator('[data-form-status]').evaluate(el=>el===document.activeElement),true);
  await page.getByLabel('Domain',{exact:true}).fill('browser.example');
  await page.getByLabel('Domain',{exact:true}).press('Enter');
  await page.locator('.domain-row summary').filter({hasText:'browser.example'}).waitFor();
  assert.equal(new URL(page.url()).searchParams.get('view'),'domains');
 });
 await check('sign out and sign in update public navigation on tab-return events',async()=>{
  const publicPage=await context.newPage();publicPage.setDefaultTimeout(10000);
  await publicPage.addInitScript(()=>{window.navigationEvents=[];for(const name of ['focus','blur','pageshow','visibilitychange'])window.addEventListener(name,()=>window.navigationEvents.push({name,hidden:document.hidden}));});
  try{
   await publicPage.goto(origin+'/guide');await publicPage.getByRole('link',{name:'Your workspaces',exact:true}).waitFor();
   await page.bringToFront();await page.getByRole('button',{name:'Sign out',exact:true}).click();
   await page.waitForURL('**/human/login');await publicPage.bringToFront();
   // Automation keeps both pages visible; exercise the tab-return handler.
   await publicPage.evaluate(()=>window.dispatchEvent(new Event('focus')));
   await publicPage.getByRole('navigation',{name:'Main',exact:true}).getByRole('link',{name:'Sign in',exact:true}).waitFor();
   await page.bringToFront();await page.getByRole('button',{name:'Sign in with a passkey',exact:true}).click();
   await page.waitForURL('**/human/workspaces');await publicPage.bringToFront();
   // Automation keeps both pages visible; exercise the tab-return handler.
   await publicPage.evaluate(()=>window.dispatchEvent(new Event('focus')));
   await publicPage.getByRole('link',{name:'Your workspaces',exact:true}).waitFor();
  }catch(error){console.error(JSON.stringify({events:await publicPage.evaluate(()=>window.navigationEvents),status:await(await context.request.get(origin+'/session-status')).json()}));throw error;}finally{await publicPage.close();await page.bringToFront();}
 });
 await check('recover with Enter after losing the authenticator',async()=>{
  await page.getByRole('button',{name:'Sign out',exact:true}).click();
  await page.waitForURL('**/human/login');
  await cdp.send('WebAuthn.removeVirtualAuthenticator',authenticator);
  authenticator=await addAuthenticator();
  await page.getByText('Recover your account',{exact:true}).click();
  await page.getByLabel('Recovery code',{exact:true}).fill('invalid-code');
  await page.getByLabel('Recovery code',{exact:true}).press('Enter');
  await page.locator('#human-status').filter({hasText:'Invalid recovery code.'}).waitFor();
  assert.equal(await page.getByLabel('Recovery code',{exact:true}).getAttribute('aria-invalid'),'true');
  assert.equal(await page.getByLabel('Recovery code',{exact:true}).evaluate(el=>el===document.activeElement),true);
  await page.getByLabel('Recovery code',{exact:true}).fill(recovery);
  await page.getByLabel('Recovery code',{exact:true}).press('Enter');
  await page.locator('#human-recovery-result').waitFor({state:'visible'});
  assert.notEqual(await page.locator('#recovery-code').textContent(),recovery);
  await page.getByRole('button',{name:'I have saved this code'}).click();
  await page.waitForURL('**/human/login');
  await page.getByRole('button',{name:'Add a passkey',exact:true}).click();
  await page.waitForURL('**/human/workspaces');
 });
 await check('no browser errors or horizontal overflow',async()=>{
  assert.deepEqual(errors,[]);
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  await page.screenshot({path:output+'/workspace-dark-mobile.png',fullPage:true});
 });
}finally{
 await writeFile(output+'/results.json',JSON.stringify({suite:'account journeys',executed:checks.length,checks},null,2));
 await browser.close();
}
assert.equal(checks.length,11,'all account journeys must execute');
console.log(JSON.stringify({suite:'account journeys',executed:checks.length,failed:0}));
