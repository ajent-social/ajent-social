import * as pulumi from '@pulumi/pulumi';
import {writeFileSync} from 'node:fs';
import {bootstrap} from './userdata';
const [mode,out] = process.argv.slice(2);
const cfg:Record<string,string>={'aws:region':'us-west-2','ajent-infra:hostname':'dogfood.ajent.social'};
if(mode!=='foundation') Object.assign(cfg,{'ajent-infra:imageDigest':'sha256:'+'a'.repeat(64),'ajent-infra:caddyImage':'caddy@sha256:'+'b'.repeat(64),'ajent-infra:cloudflareZoneId':'test-zone','ajent-infra:openrouterSecretArn':'arn:chat-test','ajent-infra:alertEmail':'operator@example.test'});
if(['transition','rolling','cutover','apex'].includes(mode)) cfg['ajent-infra:rollingService']='true';
if(['rolling','cutover','apex'].includes(mode)) cfg['ajent-infra:retireLegacyHost']='true';
if(['cutover','apex'].includes(mode)) cfg['ajent-infra:canonicalHostname']='ajent.social';
if(mode==='cutover') {cfg['ajent-infra:hostname']='ajent.social'; cfg['ajent-infra:certificateArn']='arn:apex-test';}
process.env.PULUMI_CONFIG=JSON.stringify(cfg);
const resources:any[]=[];
pulumi.runtime.setMocks({newResource(a){resources.push({type:a.type,name:a.name,inputs:a.inputs});return {id:a.name+'_id',state:{...a.inputs,arn:'arn:aws:test::123:'+a.name,address:'database.test',repositoryUrl:'123.dkr.ecr.us-west-2.amazonaws.com/ajent',publicIp:'192.0.2.1',result:'fake-secret-for-test',name:a.name,latestVersion:1,arnSuffix:a.name,identifier:a.name,dnsName:'edge.example',domainValidationOptions:[]}}},call(a){if(a.token.includes('getAvailabilityZones'))return {names:['us-west-2a','us-west-2b']};if(a.token.includes('getParameter'))return {value:'ami-test'};return a.inputs;}},'ajent-infra','test',false);
pulumi.runtime.runInPulumiStack(async()=>await import('./index'));
process.on('beforeExit',()=>{writeFileSync(out+'/'+mode+'-resources.json',JSON.stringify(resources.filter(r=>r.type!=='pulumi:pulumi:Stack').sort((a,b)=>(a.type+a.name)<(b.type+b.name)?-1:1),null,2)+'\n');});
if(['legacy','rolling','cutover'].includes(mode)) {
 const c:any={};
 if(mode==='cutover') {c.canonicalHostname='ajent.social';c.legacyOrigin='https://dogfood.ajent.social';}
 if(mode!=='legacy') c.openrouterSecretArn='arn:chat-test';
 Object.assign(c,{hostname:'dogfood.ajent.social',region:'us-west-2',dbHost:'db.example',dbArn:'arn:db',ownerArn:'arn:owner',image:'registry/app@sha256:'+'a'.repeat(64),caddyImage:'caddy@sha256:'+'b'.repeat(64),logGroup:'logs',stack:'dogfood'});
 if(mode!=='legacy')c.behindLoadBalancer=true;
 writeFileSync(out+'/'+mode+'.sh',bootstrap(c));
}
