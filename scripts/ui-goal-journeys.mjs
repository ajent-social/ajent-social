// Synthetic local human/agent workflow through the production HTTP server.
import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost')throw Error('Use an isolated localhost server.');
const output=(process.env.AJENT_BROWSER_OUTPUT||'artifacts')+'/goals';await mkdir(output,{recursive:true});
const browser=await chromium.launch();const context=await browser.newContext({viewport:{width:390,height:844},reducedMotion:'reduce'});const page=await context.newPage();page.setDefaultTimeout(10000);
await context.addInitScript({path:'node_modules/axe-core/axe.min.js'});
const checks=[],errors=[];page.on('pageerror',e=>errors.push(e.message));async function check(name,fn){await fn();checks.push(name);console.log('PASS '+name);}
const cdp=await context.newCDPSession(page);await cdp.send('WebAuthn.enable');await cdp.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}});
const key=`aj_${randomBytes(8).toString('hex')}_${randomBytes(32).toString('base64url')}`;let goalURL,goalID,workspace;
async function api(method,path,data,operation){const response=await context.request.fetch(origin+path,{method,headers:{Authorization:'Bearer '+key,...(operation?{'Idempotency-Key':operation}:{})},data});assert.ok(response.ok(),`${method} ${path}: ${response.status()} ${await response.text()}`);return response.json();}
async function update(action,body,evidence=''){const current=await api('GET','/v1/goals/'+goalID);return api('POST','/v1/goals/'+goalID+'/events',{version:current.goal.version,action,body,evidence,next_step:action==='ask'?'Waiting for your decision':'Verify the recovery flow'},randomBytes(8).toString('hex'));}
try{
 await check('create a human account and link an agent',async()=>{
  await page.goto(origin+'/human/login');await page.getByLabel('Your name',{exact:true}).fill('Goal Journey');await page.getByLabel('Your name',{exact:true}).press('Enter');await page.locator('#human-recovery-result').waitFor({state:'visible'});await page.getByRole('button',{name:'I have saved this code'}).click();await page.waitForURL('**/human/workspaces');
  const response=await context.request.post(origin+'/v1/installations/link',{headers:{Authorization:'Bearer '+key},data:{handle:'agent-'+randomBytes(4).toString('hex'),project_scope:'project:'+randomBytes(32).toString('hex')}});assert.equal(response.status(),200);const link=await response.json();await page.goto(link.url);await page.locator('#installation-consent').waitFor({state:'visible'});await page.locator('input[name="name"]').fill('Goal journey workspace');await page.locator('input[name="confirm"]').check();await page.getByRole('button',{name:'Approve installation',exact:true}).click();await page.waitForURL('**/human/workspaces?**');workspace=new URL(page.url()).searchParams.get('workspace');
  const me=await api('GET','/v1/me');assert.ok(!me.handle.startsWith('agent-'),'generated name should be playful');
 });
 await check('assign a goal with explicit outcome and scope',async()=>{
  await page.getByRole('link',{name:'Goals & updates',exact:true}).click();await page.getByRole('link',{name:'New goal',exact:true}).click();await page.getByLabel('Goal title',{exact:true}).fill('Recover interrupted onboarding');await page.getByLabel('Outcome',{exact:true}).fill('A new user can safely retry an interrupted sign-in.');await page.getByLabel('Acceptance criteria',{exact:true}).fill('The recovery regression check passes.');await page.getByLabel('Scope and authority',{exact:true}).fill('Code changes and tests only. Ask before deployment.');await page.getByRole('button',{name:'Assign goal',exact:true}).click();await page.waitForURL(/\/human\/goals\/[a-f0-9-]+/);goalURL=page.url().split('?')[0];goalID=new URL(goalURL).pathname.split('/').pop();await page.getByText('No agent update yet. Assignment has not been acknowledged.',{exact:true}).waitFor();
 });
 await check('receive and accept are distinct from working',async()=>{assert.equal((await update('receive','Received the assignment')).state,'received');assert.equal((await update('accept','I will own the recovery fix')).state,'accepted');assert.equal((await update('start','Reproducing the interruption')).state,'working');});
 await check('human receives a blocker and replies with a decision',async()=>{
  await update('ask','Should the retry preserve the selected workspace?');await page.goto(origin+'/human/goals?workspace='+workspace+'&tab=attention');await page.getByRole('link',{name:'Recover interrupted onboarding',exact:true}).click();await page.getByLabel('Message',{exact:true}).fill('Yes. Preserve the selected workspace.');await page.getByLabel('Action',{exact:true}).selectOption('decision');await page.getByRole('button',{name:'Send to agent',exact:true}).click();await page.waitForURL('**/human/goals/'+goalID+'?notice=**');const current=await api('GET','/v1/goals/'+goalID);assert.equal(current.events[0].actor_kind,'human');assert.equal(current.events[0].body,'Yes. Preserve the selected workspace.');await update('start','Applying the workspace decision');
 });
 await check('draft survives reload and is cleared only after successful send',async()=>{
  await page.goto(goalURL);await page.getByLabel('Message',{exact:true}).fill('Draft to keep across a refresh');page.once('dialog',d=>d.accept());await page.reload();assert.equal(await page.getByLabel('Message',{exact:true}).inputValue(),'Draft to keep across a refresh');await page.getByRole('button',{name:'Send to agent',exact:true}).click();await page.waitForURL('**/human/goals/'+goalID+'?notice=**');assert.equal(await page.getByLabel('Message',{exact:true}).inputValue(),'');
 });
 await check('pause is pending until acknowledged',async()=>{
  await page.getByLabel('Message',{exact:true}).fill('Pause while I review the scope');await page.getByLabel('Action',{exact:true}).selectOption('pause');await page.getByRole('button',{name:'Send to agent',exact:true}).click();await page.waitForURL('**/human/goals/'+goalID+'?notice=**');assert.equal((await api('GET','/v1/goals/'+goalID)).goal.state,'pause_requested');await update('paused','Paused at a safe boundary');await page.goto(goalURL);await page.getByLabel('Message',{exact:true}).fill('Resume the agreed work');await page.getByLabel('Action',{exact:true}).selectOption('resume');await page.getByRole('button',{name:'Send to agent',exact:true}).click();await page.waitForURL('**/human/goals/'+goalID+'?notice=**');await update('start','Resumed');
 });
 await check('completion requires agent evidence and human acceptance',async()=>{
  await update('ready','The recovery check passes','Synthetic local regression check passed.');await page.goto(goalURL);await page.getByLabel('Message',{exact:true}).fill('Evidence reviewed; accepted');await page.getByLabel('Action',{exact:true}).selectOption('complete');await page.getByRole('button',{name:'Send to agent',exact:true}).click();await page.waitForURL('**/human/goals/'+goalID+'?notice=**');assert.equal((await api('GET','/v1/goals/'+goalID)).goal.state,'completed');
 });
 await check('schedule a report in the human timezone',async()=>{
  await page.goto(origin+'/human/goals?workspace='+workspace+'&tab=updates');await page.getByLabel('Timezone',{exact:true}).fill('America/Los_Angeles');await page.getByLabel('Hour (0–23)',{exact:true}).fill('9');await page.getByRole('button',{name:'Save schedule',exact:true}).click();await page.waitForURL('**/human/goals?**');assert.equal(await page.getByLabel('Timezone',{exact:true}).inputValue(),'America/Los_Angeles');
 });
 for(const width of [390,1024,1440,2880])await check('goal detail layout '+width,async()=>{await page.setViewportSize({width,height:1000});await page.goto(goalURL);assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:output+'/goal-'+width+'.png',fullPage:true});});
 await check('keyboard reaches the reply control',async()=>{await page.getByLabel('Message',{exact:true}).focus();await page.keyboard.press('Tab');assert.equal(await page.getByLabel('Action',{exact:true}).evaluate(el=>el===document.activeElement),true);});
 await check('goal pages have no serious accessibility violations',async()=>{
  for(const path of [goalURL,origin+'/human/goals?workspace='+workspace+'&tab=new',origin+'/human/goals?workspace='+workspace+'&tab=updates']){
   await page.goto(path);
   const result=await page.evaluate(()=>axe.run(document,{runOnly:{type:'tag',values:['wcag2a','wcag2aa','wcag21aa']}}));
   assert.deepEqual(result.violations.filter(v=>['critical','serious'].includes(v.impact)).map(v=>({id:v.id,targets:v.nodes.map(n=>n.target)})),[]);
  }
 });
 assert.deepEqual(errors,[]);await writeFile(output+'/results.json',JSON.stringify({checks,executed:checks.length,composition:'real HTTP server and PostgreSQL; synthetic human WebAuthn and agent credentials'},null,2));
}catch(error){await writeFile(output+'/failure.html',await page.content());await page.screenshot({path:output+'/failure.png',fullPage:true});throw error;}finally{await browser.close();}
