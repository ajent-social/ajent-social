// Run against a disposable database through a locally built ajentd. Requires Playwright + Chrome.
import {chromium} from 'playwright';
import {randomBytes} from 'node:crypto';
import {mkdirSync,writeFileSync} from 'node:fs';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin || new URL(origin).hostname!=='localhost')throw Error('AJENT_BROWSER_ORIGIN must use localhost and a disposable database.');
const out=process.env.AJENT_BROWSER_OUTPUT||'artifacts/conversion-journeys';mkdirSync(out,{recursive:true});
const results=[];const check=(name,ok)=>{if(!ok)throw Error(name);results.push({name,status:'PASS'});writeFileSync(out+'/results.json',JSON.stringify(results,null,2));};
const browser=await chromium.launch({executablePath:process.env.CHROME_PATH||undefined,headless:true});
try{
 const context=await browser.newContext();const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
 for(const width of [390,1024,1440,2880]){
  await page.setViewportSize({width,height:1000});
  for(const path of ['/','/start','/join','/domains','/explore']){
   await page.goto(origin+path);await page.evaluate(()=>document.fonts.ready);await page.emulateMedia({reducedMotion:'reduce'});
   check(`layout ${path} at ${width}`,await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth && !!document.querySelector('h1')));
   await page.screenshot({path:out+'/'+(path==='/'?'home':path.slice(1))+'-'+width+'.png',fullPage:true});
  }
 }
 await page.setViewportSize({width:1440,height:1000});await page.goto(origin+'/');
 await page.getByRole('link',{name:'See how it works',exact:true}).click();check('demo anchor preserves URL',page.url().endsWith('#how-it-works'));
 check('example is labeled',await page.getByText('Illustrative example · Not live network activity or a measured result.').isVisible());
 await page.getByRole('link',{name:'Get started with your agent',exact:true}).first().click();
 await page.waitForURL('**/start');
 const cdp=await context.newCDPSession(page);await cdp.send('WebAuthn.enable');await cdp.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}});
 await page.getByLabel('Your name').fill('Conversion browser test');await page.getByLabel('Your name').press('Enter');
 await page.locator('#human-recovery-result').waitFor({state:'visible'});
 check('account waits for recovery acknowledgment',page.url().endsWith('/start') && await page.locator('#recovery-code').textContent());
 await page.getByRole('button',{name:'I have saved this code'}).click();await page.waitForURL('**/join');
 check('account continues to agent setup',await page.getByText('Signed in as Conversion browser test.',{exact:false}).isVisible());
 await context.grantPermissions(['clipboard-read','clipboard-write']);await page.getByRole('button',{name:'Copy setup prompt',exact:true}).click();
 check('setup prompt copies real origin and consent instruction',await page.evaluate(async()=>{const t=await navigator.clipboard.readText();return t.includes(location.origin+'/skill.md')&&t.includes('approval link')}));
 await page.evaluate(()=>Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async()=>{throw Error('denied')}}}));
 await page.getByRole('button',{name:'Copy setup prompt',exact:true}).click();check('clipboard denial offers manual copy',await page.getByRole('status').textContent().then(s=>s.includes('Select and copy')));
 const key=()=>`aj_${randomBytes(8).toString('hex')}_${randomBytes(32).toString('base64url')}`;
 const token=key();const api=async(method,path,data,t=token)=>{const r=await context.request.fetch(origin+path,{method,data,headers:{Authorization:'Bearer '+t,'Idempotency-Key':randomBytes(8).toString('hex')}});if(!r.ok())throw Error(`${path}: ${r.status()} ${await r.text()}`);return r.json()};
 const link=await api('POST','/v1/installations/link',{handle:'browser-'+randomBytes(4).toString('hex'),project_scope:'project:'+'d'.repeat(64)});
 await page.goto(link.url);await page.locator('#installation-consent').waitFor({state:'visible'});
 check('consent shows matching installer code',(await page.locator('#installation-code').textContent())===link.code);
 await page.getByLabel('New workspace name').fill('Private conversion workspace');await page.getByRole('checkbox').check();await page.getByRole('button',{name:'Approve installation',exact:true}).click();await page.waitForURL('**/human/workspaces?*');
 check('approved installation shows first finding prompt',await page.locator('#first-finding-prompt').isVisible());
 check('pending installation proof is cleared',await page.evaluate(()=>!sessionStorage.getItem('ajent-pending-installation')));
 const agentA=key(),agentB=key();const a=await api('POST','/v1/fleet',{profile:'conversion-a',token:agentA});const b=await api('POST','/v1/fleet',{profile:'conversion-b',token:agentB});
 check('agents share a private workspace',a.group_id===b.group_id && !!a.group_id);
 await api('POST','/v1/posts',{kind:'finding',title:'Conversion private retrieval proof',body:'Synthetic local verification: a second agent can retrieve this private group finding.',group_id:a.group_id},agentA);
 const found=await api('GET','/v1/search?q=Conversion%20private%20retrieval%20proof',undefined,agentB);
 check('second agent retrieves private finding',JSON.stringify(found).includes('Synthetic local verification'));
 await page.reload();check('human sees resulting agent activity',await page.getByText('Conversion private retrieval proof',{exact:true}).isVisible());
 for(const width of [390,1024,1440,2880]){await page.setViewportSize({width,height:1000});check(`workspace layout at ${width}`,await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));await page.screenshot({path:out+'/workspace-'+width+'.png',fullPage:true});}
 await page.goto(origin+'/explore');check('private finding does not appear on public web',!(await page.locator('body').textContent()).includes('Conversion private retrieval proof'));

 // Pending installation approval must take precedence over the new account-first destination.
 const second=await browser.newContext();const pending=await second.newPage();
 pending.on('pageerror',e=>errors.push(e.message));
 const pendingLink=await api('POST','/v1/installations/link',{handle:'pending-'+randomBytes(4).toString('hex'),project_scope:'project:'+'e'.repeat(64)},key());
 await pending.goto(pendingLink.url);await pending.goto(origin+'/start');
 const secondCDP=await second.newCDPSession(pending);await secondCDP.send('WebAuthn.enable');await secondCDP.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}});
 await pending.getByLabel('Your name').fill('Pending approval test');await pending.getByLabel('Your name').press('Enter');
 await pending.locator('#human-recovery-result').waitFor({state:'visible'});await pending.getByRole('button',{name:'I have saved this code'}).click();
 await pending.waitForURL('**/human/link');await pending.locator('#installation-consent').waitFor({state:'visible'});
 check('pending consent takes precedence after signup',(await pending.locator('#installation-code').textContent())===pendingLink.code);
 await pending.getByRole('link',{name:'Cancel',exact:true}).click();
 check('cancel clears pending consent',await pending.evaluate(()=>!sessionStorage.getItem('ajent-pending-installation')));
 await second.close();
 await page.goto(origin+'/');await page.setViewportSize({width:390,height:844});await page.emulateMedia({colorScheme:'dark',reducedMotion:'reduce'});
 await page.keyboard.press('Tab');check('keyboard exposes skip link',await page.getByRole('link',{name:'Skip to content',exact:true}).evaluate(e=>e===document.activeElement));
 check('reduced motion leaves hero content visible',await page.getByRole('heading',{level:1}).isVisible());
 await page.screenshot({path:out+'/home-dark-reduced-390.png',fullPage:true});
 check('no uncaught browser errors',errors.length===0);
 console.log(JSON.stringify({checks:results.length,status:'PASS'}));
}finally{await browser.close()}
