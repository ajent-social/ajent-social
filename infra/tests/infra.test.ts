import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as pulumi from '@pulumi/pulumi';
import { bootstrap } from '../userdata';
import { execFileSync } from 'node:child_process';

const resources: pulumi.runtime.MockResourceArgs[]=[];
process.env.PULUMI_CONFIG=JSON.stringify({'aws:region':'us-west-2','ajent-infra:imageDigest':'sha256:'+'a'.repeat(64),'ajent-infra:caddyImage':'caddy@sha256:'+'b'.repeat(64),'ajent-infra:cloudflareZoneId':'test-zone','ajent-infra:rollingService':'true','ajent-infra:openrouterSecretArn':'arn:chat-test'});
pulumi.runtime.setMocks({newResource(args){resources.push(args); return {id:args.name+'_id',state:{...args.inputs,arn:'arn:aws:test::123:'+args.name,address:'database.test',repositoryUrl:'123.dkr.ecr.us-west-2.amazonaws.com/ajent',publicIp:'192.0.2.1',result:'fake-secret-for-test',name:args.name,latestVersion:1,arnSuffix:args.name}};},call(args){if(args.token.includes('getAvailabilityZones'))return {names:['us-west-2a','us-west-2b']};if(args.token.includes('getParameter'))return {value:'ami-test'};return args.inputs;}},'ajent-infra','test',false);

test('deployment keeps database private, secrets out of user data, and DNS unproxied',async()=>{
    const stack=await import('../index');
    await new Promise(resolve=>pulumi.all([stack.instanceId,stack.publicIp,stack.repositoryUrl,stack.autoscalingGroup,stack.loadBalancerDns]).apply(resolve));
    const one=(type:string)=>{const r=resources.find(r=>r.type===type);assert.ok(r,type);return r.inputs;};
    const db=one('aws:rds/instance:Instance');
    assert.equal(db.publiclyAccessible,false);assert.equal(db.storageEncrypted,true);assert.equal(db.deletionProtection,true);assert.equal(db.skipFinalSnapshot,false);assert.equal(db.backupRetentionPeriod,7);
    const host=one('aws:ec2/instance:Instance');
    assert.equal(host.metadataOptions.httpTokens,'required');assert.equal(host.rootBlockDevice.encrypted,true);
    assert.ok(!host.userData.includes('fake-secret-for-test'));
    const groups=resources.filter(r=>r.type==='aws:ec2/securityGroup:SecurityGroup');
    assert.ok(groups.every(r=>!r.inputs.ingress.some((i:any)=>i.fromPort===22)));
    const dbGroup=groups.find(r=>r.name==='database')!;
    assert.equal(dbGroup.inputs.ingress[0].fromPort,5432);assert.equal(dbGroup.inputs.ingress[0].cidrBlocks,undefined);
    // DNS registration can settle after the exported IP.
    await new Promise(resolve=>setTimeout(resolve,100));
    assert.equal(one('cloudflare:index/dnsRecord:DnsRecord').proxied,false);
    const fleet=one('aws:autoscaling/group:Group');
    assert.equal(fleet.minSize,2);assert.equal(fleet.maxSize,4);
    assert.equal(fleet.instanceRefresh.preferences.minHealthyPercentage,100);
    assert.equal(fleet.instanceRefresh.preferences.maxHealthyPercentage,150);
    assert.equal(fleet.instanceRefresh.preferences.autoRollback,true);
    assert.equal(fleet.healthCheckType,'ELB');assert.equal(fleet.vpcZoneIdentifiers.length,2);
    assert.equal(one('aws:lb/targetGroup:TargetGroup').deregistrationDelay,30);
    const fleetSG=groups.find(r=>r.name==='fleet')!;
    assert.equal(fleetSG.inputs.ingress[0].fromPort,8080);
    assert.equal(fleetSG.inputs.ingress[0].cidrBlocks,undefined);
    assert.equal(one('aws:acm/certificate:Certificate').validationMethod,'DNS');
});

test('bootstrap shell and every embedded Python program parse, without plaintext secrets',()=>{
    const script=bootstrap({hostname:'dogfood.ajent.social',region:'us-west-2',dbHost:'db.example',dbArn:'arn:db',ownerArn:'arn:owner',image:'registry/app@sha256:'+'a'.repeat(64),caddyImage:'caddy@sha256:'+'b'.repeat(64),logGroup:'logs',stack:'dogfood',behindLoadBalancer:true,openrouterSecretArn:'arn:chat-test'});
    assert.ok(Buffer.byteLength(script)<16384,'EC2 user data limit');
    execFileSync('bash',['-n'],{input:script});
    const embedded=[...script.matchAll(/printf '%s' '([^']+)' \| base64 -d > (\S+)/g)];
    assert.equal(embedded.length,9);
    for(const [,encoded,path] of embedded){
        const body=Buffer.from(encoded,'base64').toString();
        if(path.endsWith('.py'))execFileSync('python3',['-c','import ast,sys; ast.parse(sys.stdin.read())'],{input:body});
    }
    const refresh=Buffer.from(embedded.find(m=>m[2].endsWith('refresh.py'))![1],'base64').toString();
    assert.match(refresh,/OPENROUTER_API_KEY/);assert.match(refresh,/openrouterSecretArn/);
    assert.match(refresh,/10\.87\.1\.0\/24,10\.87\.2\.0\/24/);assert.doesNotMatch(script,/enable --now ajent-app ajent-proxy/);assert.match(refresh,/sslmode=verify-full/);assert.match(refresh,/os.umask\(0o077\)/);
});
