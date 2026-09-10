import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as pulumi from '@pulumi/pulumi';

const resources:pulumi.runtime.MockResourceArgs[]=[];
process.env.PULUMI_CONFIG=JSON.stringify({'aws:region':'us-west-2','ajent-infra:imageDigest':'sha256:'+'a'.repeat(64),'ajent-infra:caddyImage':'caddy@sha256:'+'b'.repeat(64)});
pulumi.runtime.setMocks({newResource(args){resources.push(args);return {id:args.name,state:{...args.inputs,arn:'arn:test:'+args.name,name:args.name,identifier:args.name,result:'test-secret',repositoryUrl:'registry/app'}};},call(args){if(args.token.includes('getAvailabilityZones'))return {names:['us-west-2a','us-west-2b']};if(args.token.includes('getParameter'))return {value:'ami-test'};return args.inputs;}},'ajent-infra','disabled-test',false);

test('unconfigured improvement worker creates no credential access or worker alarms',async()=>{
    const stack=await import('../index');
    await new Promise(resolve=>pulumi.all([stack.repositoryUrl,stack.logGroupName,stack.alertTopicArn,stack.databaseIdentifier,stack.instanceId]).apply(resolve));
    await pulumi.runtime.disconnect();
    assert.ok(resources.length>0);
    assert.equal(resources.filter(r=>r.name.startsWith('improvement-worker-')).length,0);
    const runtime=resources.find(r=>r.name==='runtime'&&r.type==='aws:iam/rolePolicy:RolePolicy')!;
    const secretAccess=JSON.parse(runtime.inputs.policy).Statement.find((s:any)=>s.Action.includes('secretsmanager:GetSecretValue'));
    assert.deepEqual(secretAccess.Resource,['arn:test:database-secret','arn:test:owner-secret']);
});
