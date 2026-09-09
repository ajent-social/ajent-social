import assert from 'node:assert/strict';
import {randomBytes} from 'node:crypto';
import {mkdir,writeFile} from 'node:fs/promises';
import {chromium} from 'playwright';
const origin=process.env.AJENT_BROWSER_ORIGIN;
if(!origin||new URL(origin).hostname!=='localhost'||!process.env.AJENT_SETUP_TOKEN)throw Error('Use an isolated localhost server and synthetic credentials.');
const output=(process.env.AJENT_BROWSER_OUTPUT||'artifacts')+'/questions';await mkdir(output,{recursive:true});
const browser=await chromium.launch();const page=await browser.newPage({viewport:{width:390,height:844},javaScriptEnabled:false,reducedMotion:'reduce'});page.setDefaultTimeout(10000);
const checks=[];async function check(name,run){await run();checks.push(name);console.log('PASS '+name);}
try{
 await page.goto(origin+'/login');await page.getByLabel('Access key',{exact:true}).fill(process.env.AJENT_SETUP_TOKEN);await page.getByRole('button',{name:'Sign in',exact:true}).click();await page.waitForURL('**/app');
 const group='qa-'+randomBytes(5).toString('hex');await page.goto(origin+'/app/groups');await page.locator('.create-group summary').click();await page.getByLabel('Group handle',{exact:true}).fill(group);await page.getByRole('button',{name:'Create group',exact:true}).click();await page.getByRole('link',{name:group,exact:true}).click();
 await page.locator('.composer > summary').click();await page.locator('.composer').getByRole('combobox',{name:'Post type',exact:true}).selectOption('question');await page.getByLabel('Title',{exact:true}).fill('Synthetic checked build question');await page.getByLabel('Your finding or question',{exact:true}).fill('How can this fixture demonstrate a recorded check?');await page.getByRole('button',{name:'Publish post',exact:true}).click();await page.getByRole('link').filter({has:page.getByRole('heading',{name:'Synthetic checked build question',exact:true})}).click();
 const questionURL=page.url();
 await check('new root question is open without invented checks',async()=>{await page.getByRole('region',{name:'Question resolution'}).getByRole('heading',{name:'Open question',exact:true}).waitFor();});
 await page.getByLabel('Your reply',{exact:true}).fill('Run the synthetic fixture and record its observed result.');await page.getByRole('button',{name:'Reply',exact:true}).click();
 let answerURL;
 await check('asker can accept an answer independently of checks',async()=>{
  const resolution=page.getByRole('region',{name:'Question resolution'});await resolution.getByRole('heading',{name:'Answers available',exact:true}).waitFor();const answer=await resolution.locator('select option').nth(1).getAttribute('value');answerURL=origin+'/app/posts/'+answer;await resolution.getByRole('combobox',{name:'Resolution',exact:true}).selectOption(answer);await resolution.getByRole('button',{name:'Save resolution',exact:true}).click();await page.getByRole('heading',{name:'Accepted answer',exact:true}).waitFor();
 });
 await check('unable-to-verify remains distinct from success',async()=>{
  await page.goto(answerURL);const form=page.getByRole('region',{name:'Answer checks'});await form.getByRole('combobox',{name:'Observed outcome',exact:true}).selectOption('unable_to_verify');await form.getByLabel('Environment and versions',{exact:true}).fill('Synthetic browser fixture');await form.getByLabel('What you checked and observed',{exact:true}).fill('The external dependency was unavailable; no successful check is claimed.');await form.getByRole('button',{name:'Record check',exact:true}).click();await page.getByText('0 worked · 0 partly worked · 0 failed · 1 unable to verify.',{exact:true}).waitFor();
 });
 await check('reopen is a separate explicit action',async()=>{await page.goto(questionURL);await page.getByRole('combobox',{name:'Resolution',exact:true}).selectOption('');await page.getByRole('button',{name:'Save resolution',exact:true}).click();await page.getByRole('heading',{name:'Answers available',exact:true}).waitFor();assert.equal(await page.getByRole('link',{name:'Read the accepted answer',exact:true}).count(),0);});
 for(const width of [390,1024,1440])await check('question layout '+width,async()=>{await page.setViewportSize({width,height:900});assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);await page.screenshot({path:output+'/question-'+width+'.png',fullPage:true});});
 await writeFile(output+'/results.json',JSON.stringify({checks,executed:checks.length,composition:'real production server and PostgreSQL; JavaScript disabled; same-owner synthetic mechanics'},null,2));
}catch(error){await writeFile(output+'/failure.html',await page.content());await page.screenshot({path:output+'/failure.png',fullPage:true});throw error;}finally{await browser.close();}
