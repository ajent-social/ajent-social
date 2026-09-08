// Real account/link flows with denied or full browser session storage.
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost')throw Error('Use localhost and a disposable database.');
const output=resolve(process.env.AJENT_BROWSER_OUTPUT||'artifacts/storage-journeys');
await mkdir(output,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||undefined});
const checks=[];
async function check(name,run){await run();checks.push(name);console.log('PASS '+name);}
try{
 for(const mode of ['denied','full']){
  const context=await browser.newContext({viewport:{width:390,height:844},colorScheme:'dark'});
  await context.addInitScript(mode=>{
   if(mode==='denied')Object.defineProperty(window,'sessionStorage',{get(){throw new DOMException('Storage denied','SecurityError');}});
   else Storage.prototype.setItem=function(){throw new DOMException('Storage full','QuotaExceededError');};
  },mode);
  const page=await context.newPage();page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  let recovery;
  const cdp=await context.newCDPSession(page);await cdp.send('WebAuthn.enable');
  await cdp.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}});
  async function installation(){
   const key=`aj_${randomBytes(8).toString('hex')}_${randomBytes(32).toString('base64url')}`;
   const response=await context.request.post(origin+'/v1/installations/link',{headers:{Authorization:'Bearer '+key},data:{handle:'storage-'+randomBytes(5).toString('hex'),project_scope:'project:'+randomBytes(32).toString('hex')}});
   assert.equal(response.status(),200);return {key,...await response.json()};
  }
  async function approve(link){
   await page.locator('#installation-consent').waitFor({state:'visible'});
   assert.equal(new URL(page.url()).hash,'','installation secret must be removed from the address');
   assert.equal(await page.locator('#installation-code').textContent(),link.code);
   await page.locator('#link-workspace').selectOption('');
   await page.locator('input[name="name"]').fill('Storage test workspace');
   await page.locator('input[name="confirm"]').check();
   await page.getByRole('button',{name:'Approve installation',exact:true}).click();
   await page.waitForURL('**/human/workspaces?**');
   const response=await context.request.get(origin+'/v1/installations/link',{headers:{Authorization:'Bearer '+link.key}});
   assert.equal((await response.json()).status,'approved');
  }
  try{
   await check(mode+': registration reaches workspaces',async()=>{
    await page.goto(origin+'/human/login');
    await page.getByLabel('Your name',{exact:true}).fill('Storage browser test');
    await page.getByLabel('Your name',{exact:true}).press('Enter');
    await page.locator('#human-recovery-result').waitFor({state:'visible'});
    recovery=await page.locator('#recovery-code').textContent();
    await page.getByRole('button',{name:'I have saved this code'}).click();
    await page.waitForURL('**/human/workspaces');
   });
   await check(mode+': signed-in installation approval',async()=>{
    const link=await installation();await page.goto(link.url);await approve(link);
   });
   await check(mode+': installation survives passkey sign-in',async()=>{
    await page.getByRole('button',{name:'Sign out',exact:true}).click();await page.waitForURL('**/human/login');
    const link=await installation();await page.goto(link.url);
    await page.getByRole('button',{name:'Sign in with a passkey',exact:true}).click();await approve(link);
    assert.deepEqual(errors,[],'account/link flows must not throw browser errors');
   });
   await check(mode+': installation survives account recovery',async()=>{
    await page.getByRole('button',{name:'Sign out',exact:true}).click();await page.waitForURL('**/human/login');
    const link=await installation();await page.goto(link.url);
    await page.getByText('Recover your account',{exact:true}).click();
    await page.getByLabel('Recovery code',{exact:true}).fill(recovery);
    await page.getByLabel('Recovery code',{exact:true}).press('Enter');
    await page.locator('#human-recovery-result').waitFor({state:'visible'});
    await page.getByRole('button',{name:'I have saved this code'}).click();
    await page.waitForURL(url=>url.pathname==='/human/login');
    await page.getByRole('button',{name:'Add a passkey',exact:true}).click();
    await approve(link);assert.deepEqual(errors,[]);
   });
  }finally{await context.close();}
 }
}finally{await writeFile(output+'/results.json',JSON.stringify({suite:'storage journeys',executed:checks.length,checks},null,2));await browser.close();}
assert.equal(checks.length,8,'all storage journeys must execute');
console.log(JSON.stringify({suite:'storage journeys',executed:checks.length,failed:0}));
