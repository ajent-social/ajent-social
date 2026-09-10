import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as pulumi from '@pulumi/pulumi';
import { bootstrap, validateImprovementConfig } from '../userdata';
import { execFileSync } from 'node:child_process';

const resources: pulumi.runtime.MockResourceArgs[]=[];
process.env.PULUMI_CONFIG=JSON.stringify({'aws:region':'us-west-2','ajent-infra:imageDigest':'sha256:'+'a'.repeat(64),'ajent-infra:caddyImage':'caddy@sha256:'+'b'.repeat(64),'ajent-infra:cloudflareZoneId':'test-zone','ajent-infra:rollingService':'true','ajent-infra:openrouterSecretArn':'arn:chat-test','ajent-infra:improvementRepository':'ajent-social/ajent-social','ajent-infra:improvementGitHubSecretArn':'arn:aws:secretsmanager:us-west-2:123456789012:secret:improvement-test'});
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
    const runtime=resources.find(r=>r.name==='runtime'&&r.type==='aws:iam/rolePolicy:RolePolicy')!;
    const access=JSON.parse(runtime.inputs.policy).Statement.find((s:any)=>s.Action.includes('secretsmanager:GetSecretValue'));
    assert.deepEqual(access.Resource,['arn:aws:test::123:database-secret','arn:aws:test::123:owner-secret','arn:chat-test','arn:aws:secretsmanager:us-west-2:123456789012:secret:improvement-test']);
    assert.ok(!access.Resource.some((r:string)=>r.includes('*')));
    const filters=resources.filter(r=>r.type==='aws:cloudwatch/logMetricFilter:LogMetricFilter');
    const heartbeat=filters.find(r=>r.name==='improvement-worker-health')!.inputs;
    assert.equal(heartbeat.metricTransformation.value,'$.healthy');
    assert.equal(heartbeat.metricTransformation.defaultValue,undefined);
    assert.match(heartbeat.pattern,/improvement_worker/);
    const alarms=resources.filter(r=>r.type==='aws:cloudwatch/metricAlarm:MetricAlarm');
    const stalled=alarms.find(r=>r.name==='improvement-worker-stalled')!.inputs;
    assert.equal(stalled.period,60);assert.equal(stalled.evaluationPeriods,5);assert.equal(stalled.datapointsToAlarm,5);
    assert.equal(stalled.statistic,'Minimum');assert.equal(stalled.treatMissingData,'breaching');
    assert.deepEqual(stalled.alarmActions,stalled.okActions);assert.equal(stalled.alarmActions.length,1);
    const overdue=alarms.find(r=>r.name==='improvement-worker-overdue')!.inputs;
    assert.equal(overdue.threshold,900);assert.equal(overdue.statistic,'Maximum');assert.equal(overdue.treatMissingData,'notBreaching');
    assert.deepEqual(overdue.alarmActions,overdue.okActions);
    const template=one('aws:ec2/launchTemplate:LaunchTemplate');
    const launch=Buffer.from(template.userData,'base64').toString();
    const encoded=[...launch.matchAll(/printf '%s' '([^']+)' \| base64 -d > (\S+)/g)].find(m=>m[2].endsWith('config.json'))![1];
    const config=JSON.parse(Buffer.from(encoded,'base64').toString());
    assert.equal(config.improvementRepository,'ajent-social/ajent-social');
    assert.equal(config.improvementGitHubSecretArn,access.Resource.at(-1));

});

test('bootstrap shell and every embedded Python program parse, without plaintext secrets',()=>{
    const script=bootstrap({canonicalHostname:'ajent.social',hostname:'ajent.social',region:'us-west-2',dbHost:'db.example',dbArn:'arn:db',ownerArn:'arn:owner',image:'registry/app@sha256:'+'a'.repeat(64),caddyImage:'caddy@sha256:'+'b'.repeat(64),logGroup:'logs',stack:'production',behindLoadBalancer:true,openrouterSecretArn:'arn:chat-test',improvementRepository:'ajent-social/ajent-social',improvementGitHubSecretArn:'arn:aws:secretsmanager:us-west-2:123456789012:secret:improvement-test'});
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

test('improvement configuration accepts preparation without a token but rejects broad identities',()=>{
    validateImprovementConfig({});
    validateImprovementConfig({improvementRepository:'ajent-social/ajent-social'});
    for(const repo of ['', 'https://github.com/owner/repo', 'owner/repo/extra', 'owner/*', 'Owner/repo', 'owner/..']) assert.throws(()=>validateImprovementConfig({improvementRepository:repo}));
    assert.throws(()=>validateImprovementConfig({improvementGitHubSecretArn:'arn:aws:secretsmanager:us-west-2:123456789012:secret:token'}));
    assert.throws(()=>validateImprovementConfig({improvementRepository:'owner/repo',improvementGitHubSecretArn:'*'}));
});

test('invalid or unavailable worker credential cannot inject environment or stop the app',()=>{
    const config={hostname:'ajent.social',region:'us-west-2',dbHost:'db.example',dbArn:'arn:db',ownerArn:'arn:owner',image:'registry/app@sha256:'+'a'.repeat(64),caddyImage:'caddy@sha256:'+'b'.repeat(64),logGroup:'logs',stack:'production',improvementRepository:'owner/repo',improvementGitHubSecretArn:'arn:aws:secretsmanager:us-west-2:123456789012:secret:improvement-test'};
    const script=bootstrap(config);
    const encoded=[...script.matchAll(/printf '%s' '([^']+)' \| base64 -d > (\S+)/g)].find(m=>m[2].endsWith('refresh.py'))![1];
    const program=Buffer.from(encoded,'base64').toString();
    const result=execFileSync('python3',['-c',String.raw`
import contextlib,io,json,subprocess,sys
from unittest.mock import patch
source=json.load(sys.stdin)
for token in ['scoped-test-token','bad\nINJECTED=yes','bad\rvalue','bad value','',None, {'wrong':'format'}]:
    state={}
    class Writer:
        def __enter__(self):return self
        def __exit__(self,*args):return False
        def write(self,text):state['env']+=text
    class Path:
        def __init__(self,*args):pass
        def read_text(self):return json.dumps(source['config'])
        def write_text(self,text):state['env']=text
        def open(self,mode):assert mode=='a';return Writer()
        def replace(self,target):state['promoted']=True
    def output(args,**kwargs):
        if 'get-login-password' in args:return b'registry-test-password'
        arn=args[-1]
        if arn==source['config']['improvementGitHubSecretArn']:
            assert kwargs.get('timeout')==15
            assert kwargs.get('stderr')==subprocess.DEVNULL
            if token is None:raise subprocess.CalledProcessError(1,args,stderr=b'private-provider-detail')
            return json.dumps({'SecretString':token}).encode()
        return json.dumps({'SecretString':'test-value'}).encode()
    stderr=io.StringIO()
    with patch('pathlib.Path',Path),patch('subprocess.check_output',output),patch('subprocess.run'),patch('os.umask'),contextlib.redirect_stderr(stderr):
        exec(compile(source['program'],'refresh.py','exec'),{})
    assert state['promoted'] is True
    assert 'AJENT_IMPROVEMENT_REPOSITORY=owner/repo\n' in state['env']
    assert 'AJENT_DSN=' in state['env']
    assert ('AJENT_IMPROVEMENT_GITHUB_TOKEN=' in state['env'])==(token=='scoped-test-token')
    assert 'INJECTED=' not in state['env']
    assert 'private-provider-detail' not in stderr.getvalue()
print('seven credential cases passed')
`],{input:JSON.stringify({config,program})}).toString();
    assert.match(result,/seven credential cases passed/);
});
