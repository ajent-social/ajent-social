import * as pulumi from '@pulumi/pulumi';
import * as aws from '@pulumi/aws';
import * as random from '@pulumi/random';
import * as cloudflare from '@pulumi/cloudflare';
import { bootstrap } from './userdata';
import { rollingService } from './rolling';

const cfg = new pulumi.Config();
const hostname = cfg.get('hostname') ?? 'ajent.social';
if (!/^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$/.test(hostname)) throw new Error('hostname must be a lowercase DNS name');
const canonicalHostname = cfg.get('canonicalHostname');
const certificateArn = cfg.get('certificateArn');
if (canonicalHostname && canonicalHostname !== 'ajent.social') throw new Error('canonicalHostname must be ajent.social for this cutover');
const legacyOrigin = canonicalHostname ? `https://${hostname}` : undefined;
const imageDigest = cfg.get('imageDigest');
if(cfg.getBoolean('retireLegacyHost') && !cfg.getBoolean('rollingService')) throw new Error('retireLegacyHost requires rollingService');
if (imageDigest && !/^sha256:[a-f0-9]{64}$/.test(imageDigest)) throw new Error('imageDigest must be sha256:<64 hex characters>');
const caddyImage = cfg.get('caddyImage');
if (imageDigest && (!caddyImage || !/^caddy@sha256:[a-f0-9]{64}$/.test(caddyImage))) throw new Error('Set caddyImage to caddy@sha256:<digest> before enabling the host');
const region = aws.config.region;
if (!region) throw new Error('Set aws:region explicitly');
const stack = pulumi.getStack();
const tags = { Project: 'ajent', Environment: stack, ManagedBy: 'Pulumi' };
const az = aws.getAvailabilityZonesOutput({state:'available'});
const vpc = new aws.ec2.Vpc('network', {cidrBlock:'10.87.0.0/16',enableDnsHostnames:true,enableDnsSupport:true,tags});
const gateway = new aws.ec2.InternetGateway('internet',{vpcId:vpc.id,tags});
const publicSubnet = new aws.ec2.Subnet('application-subnet',{vpcId:vpc.id,cidrBlock:'10.87.1.0/24',availabilityZone:az.names.apply(n=>n[0]),mapPublicIpOnLaunch:true,tags});
const privateSubnets = [0,1].map(i=>new aws.ec2.Subnet(`database-subnet-${i}`,{vpcId:vpc.id,cidrBlock:`10.87.${10+i}.0/24`,availabilityZone:az.names.apply(n=>{if(n.length<2)throw new Error('Two AZs required');return n[i];}),tags}));
const route = new aws.ec2.RouteTable('public-routes',{vpcId:vpc.id,routes:[{cidrBlock:'0.0.0.0/0',gatewayId:gateway.id}],tags});
new aws.ec2.RouteTableAssociation('application-route',{subnetId:publicSubnet.id,routeTableId:route.id});
const appSG = new aws.ec2.SecurityGroup('application',{vpcId:vpc.id,ingress:cfg.getBoolean('retireLegacyHost') ? [] : [80,443].map(port=>({protocol:'tcp',fromPort:port,toPort:port,cidrBlocks:['0.0.0.0/0']})),egress:[{protocol:'-1',fromPort:0,toPort:0,cidrBlocks:['0.0.0.0/0']}],tags});
const dbSG = new aws.ec2.SecurityGroup('database',{vpcId:vpc.id,ingress:[{protocol:'tcp',fromPort:5432,toPort:5432,securityGroups:[appSG.id]}],tags});
const subnetGroup = new aws.rds.SubnetGroup('database',{subnetIds:privateSubnets.map(s=>s.id),tags});
const dbPassword = new random.RandomPassword('database-password',{length:48,special:false});
const ownerPassword = new random.RandomPassword('owner-key',{length:64,special:false});
const dbSecret = new aws.secretsmanager.Secret('database-secret',{recoveryWindowInDays:7,tags},{protect:true});
const ownerSecret = new aws.secretsmanager.Secret('owner-secret',{recoveryWindowInDays:7,tags},{protect:true});
const dbSecretVersion = new aws.secretsmanager.SecretVersion('database-secret-value',{secretId:dbSecret.id,secretString:dbPassword.result});
const ownerSecretVersion = new aws.secretsmanager.SecretVersion('owner-secret-value',{secretId:ownerSecret.id,secretString:ownerPassword.result});
const db = new aws.rds.Instance('database',{
    engine:'postgres',engineVersion:'16',instanceClass:'db.t4g.micro',allocatedStorage:20,maxAllocatedStorage:100,storageType:'gp3',storageEncrypted:true,
    dbName:'ajent',username:'ajent',password:dbPassword.result,port:5432,dbSubnetGroupName:subnetGroup.name,vpcSecurityGroupIds:[dbSG.id],
    publiclyAccessible:false,multiAz:false,backupRetentionPeriod:7,backupWindow:'09:00-10:00',maintenanceWindow:'sun:10:00-sun:11:00',
    deletionProtection:true,skipFinalSnapshot:false,finalSnapshotIdentifier:`ajent-${stack}-final`,autoMinorVersionUpgrade:true,copyTagsToSnapshot:true,tags,
},{protect:true});
const repository = new aws.ecr.Repository('application-images',{imageTagMutability:'IMMUTABLE',imageScanningConfiguration:{scanOnPush:true},forceDelete:false,tags});
const logs = new aws.cloudwatch.LogGroup('application-logs',{retentionInDays:30,tags});
const alerts = new aws.sns.Topic('alerts',{tags});
const openrouterSecretArn = cfg.get('openrouterSecretArn');
const alertEmail = cfg.get('alertEmail');
if(alertEmail) new aws.sns.TopicSubscription('operator',{topic:alerts.arn,protocol:'email',endpoint:alertEmail});
for (const [name,metric,threshold,comparison] of [
    ['database-storage','FreeStorageSpace',5*1024**3,'LessThanThreshold'],
    ['database-cpu','CPUUtilization',85,'GreaterThanThreshold'],
] as const) new aws.cloudwatch.MetricAlarm(name,{namespace:'AWS/RDS',metricName:metric,dimensions:{DBInstanceIdentifier:db.identifier},statistic:'Average',period:300,evaluationPeriods:3,threshold,comparisonOperator:comparison,treatMissingData:'breaching',alarmActions:[alerts.arn],okActions:[alerts.arn],tags});
let edge: ReturnType<typeof rollingService> | undefined;
let host: aws.ec2.Instance | undefined;
let address: aws.ec2.Eip | undefined;
if(imageDigest){
    const role = new aws.iam.Role('host',{assumeRolePolicy:aws.iam.assumeRolePolicyForPrincipal({Service:'ec2.amazonaws.com'}),tags});
    const ssm = new aws.iam.RolePolicyAttachment('ssm',{role:role.name,policyArn:'arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore'});
    const policy = new aws.iam.RolePolicy('runtime',{role:role.id,policy:pulumi.all([dbSecret.arn,ownerSecret.arn,repository.arn,logs.arn]).apply(([d,o,r,l])=>JSON.stringify({Version:'2012-10-17',Statement:[
        {Effect:'Allow',Action:['secretsmanager:GetSecretValue'],Resource:[d,o,...(openrouterSecretArn ? [openrouterSecretArn] : [])]},
        {Effect:'Allow',Action:['ecr:GetAuthorizationToken'],Resource:'*'},
        {Effect:'Allow',Action:['ecr:BatchGetImage','ecr:GetDownloadUrlForLayer','ecr:BatchCheckLayerAvailability'],Resource:r},
        {Effect:'Allow',Action:['logs:CreateLogStream','logs:PutLogEvents'],Resource:`${l}:*`},
        {Effect:'Allow',Action:['cloudwatch:PutMetricData'],Resource:'*',Condition:{StringEquals:{'cloudwatch:namespace':'Ajent/Production'}}},
    ]}))});
    const profile = new aws.iam.InstanceProfile('host',{role:role.name,tags});
    const ami = aws.ssm.getParameterOutput({name:'/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64'});
    if(cfg.getBoolean('rollingService')) edge = rollingService({canonicalHostname,legacyOrigin,certificateArn,openrouterSecretArn,hostname,region,stack,tags,vpc,publicSubnet,route,appSG,profile,ami:ami.value,repository,logs,db,dbSecret,ownerSecret,imageDigest,caddyImage:caddyImage!,alerts,dependencies:[policy,ssm,dbSecretVersion,ownerSecretVersion,gateway,route]});
    if(!cfg.getBoolean('retireLegacyHost')) {
    host = new aws.ec2.Instance('application-host',{ami:ami.value,instanceType:'t4g.small',subnetId:publicSubnet.id,vpcSecurityGroupIds:[appSG.id],iamInstanceProfile:profile.name,
        associatePublicIpAddress:true,metadataOptions:{httpTokens:'required',httpEndpoint:'enabled',httpPutResponseHopLimit:2},
        rootBlockDevice:{encrypted:true,volumeType:'gp3',volumeSize:20},userDataReplaceOnChange:true,
        userData:pulumi.all([db.address,dbSecret.arn,ownerSecret.arn,repository.repositoryUrl,logs.name]).apply(([dbHost,dbArn,ownerArn,repo,logGroup])=>bootstrap({openrouterSecretArn,hostname,region,dbHost,dbArn,ownerArn,image:`${repo}@${imageDigest}`,caddyImage:caddyImage!,logGroup,stack})),tags,
    },{dependsOn:[policy,ssm,dbSecretVersion,ownerSecretVersion,gateway,route]});
    address = new aws.ec2.Eip('application-address',{domain:'vpc',instance:host.id,tags});
    new aws.cloudwatch.MetricAlarm('host-status',{namespace:'AWS/EC2',metricName:'StatusCheckFailed',dimensions:{InstanceId:host.id},statistic:'Maximum',period:60,evaluationPeriods:2,threshold:1,comparisonOperator:'GreaterThanOrEqualToThreshold',treatMissingData:'breaching',alarmActions:[alerts.arn],tags});
    }
    for(const [name,metric,threshold,comparison] of [['http-health','Healthy',1,'LessThanThreshold'],['host-disk','DiskUsedPercent',85,'GreaterThanThreshold']] as const)
        new aws.cloudwatch.MetricAlarm(name,{namespace:'Ajent/Production',metricName:metric,dimensions:{Stack:stack},statistic:metric==='DiskUsedPercent'?'Maximum':'Minimum',period:60,evaluationPeriods:3,threshold,comparisonOperator:comparison,treatMissingData:'breaching',alarmActions:[alerts.arn],okActions:[alerts.arn],tags});
    const zone = cfg.get('cloudflareZoneId');
    if(zone && (edge || address)) new cloudflare.DnsRecord('production-dns',{zoneId:zone,name:hostname,type:edge?'CNAME':'A',content:edge?edge.loadBalancer.dnsName:address!.publicIp,ttl:300,proxied:false,comment:'Ajent service; DNS-only'});
}
export const serviceUrl = `https://${hostname}`;
export const repositoryUrl = repository.repositoryUrl;
export const databaseIdentifier = db.identifier;
export const ownerSecretArn = ownerSecret.arn;
export const instanceId = host?.id;
export const publicIp = address?.publicIp;
export const alertTopicArn = alerts.arn;
export const logGroupName = logs.name;
export const deploymentStage = edge ? 'rolling-service-configured-verify-refresh-before-release' : imageDigest ? 'host-configured-not-health-verified' : 'foundation-only';

export const loadBalancerDns = edge?.loadBalancer.dnsName;
export const certificateValidation = edge?.certificate.domainValidationOptions;
export const autoscalingGroup = edge?.group.name;
