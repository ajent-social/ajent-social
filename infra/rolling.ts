import * as pulumi from '@pulumi/pulumi';
import * as aws from '@pulumi/aws';
import { bootstrap } from './userdata';
interface Config {
 openrouterSecretArn?:string;
 canonicalHostname?:string; legacyOrigin?:string; certificateArn?:string;
 hostname:string; region:string; stack:string; tags:Record<string,string>;
 vpc:aws.ec2.Vpc; publicSubnet:aws.ec2.Subnet; route:aws.ec2.RouteTable;
 appSG:aws.ec2.SecurityGroup; profile:aws.iam.InstanceProfile; ami:pulumi.Input<string>;
 repository:aws.ecr.Repository; logs:aws.cloudwatch.LogGroup; db:aws.rds.Instance;
 dbSecret:aws.secretsmanager.Secret; ownerSecret:aws.secretsmanager.Secret;
 imageDigest:string; caddyImage:string; alerts:aws.sns.Topic; dependencies:pulumi.Resource[];
}
export function rollingService(c:Config) {
 const az=aws.getAvailabilityZonesOutput({state:'available'});
 const subnet=new aws.ec2.Subnet('application-subnet-1',{vpcId:c.vpc.id,cidrBlock:'10.87.2.0/24',availabilityZone:az.names.apply(n=>n[1]),mapPublicIpOnLaunch:true,tags:c.tags});
 const route=new aws.ec2.RouteTableAssociation('application-route-1',{subnetId:subnet.id,routeTableId:c.route.id});
 const edgeSG=new aws.ec2.SecurityGroup('edge',{vpcId:c.vpc.id,ingress:[80,443].map(port=>({protocol:'tcp',fromPort:port,toPort:port,cidrBlocks:['0.0.0.0/0']})),egress:[{protocol:'tcp',fromPort:8080,toPort:8080,cidrBlocks:['10.87.1.0/24','10.87.2.0/24']}],tags:c.tags});
 const fleetSG=new aws.ec2.SecurityGroup('fleet',{vpcId:c.vpc.id,ingress:[{protocol:'tcp',fromPort:8080,toPort:8080,securityGroups:[edgeSG.id]}],egress:[],tags:c.tags});
 const loadBalancer=new aws.lb.LoadBalancer('public-edge',{loadBalancerType:'application',internal:false,subnets:[c.publicSubnet.id,subnet.id],securityGroups:[edgeSG.id],dropInvalidHeaderFields:true,enableDeletionProtection:true,tags:c.tags});
 const target=new aws.lb.TargetGroup('application-fleet',{vpcId:c.vpc.id,port:8080,protocol:'HTTP',targetType:'instance',deregistrationDelay:30,healthCheck:{path:'/healthz',port:'traffic-port',protocol:'HTTP',matcher:'200',interval:15,timeout:5,healthyThreshold:2,unhealthyThreshold:2},tags:c.tags});
 // Keep the original managed certificate in state while a separately issued
 // apex certificate is cut over. This avoids destroying the live listener's
 // certificate before the replacement has been validated by ACM.
 const certificate=new aws.acm.Certificate('public-tls',{domainName:c.certificateArn?['dog','food.ajent.social'].join(''):c.hostname,subjectAlternativeNames:c.certificateArn?[]:(c.canonicalHostname==='ajent.social'?['www.ajent.social']:[]),validationMethod:'DNS',tags:c.tags},{protect:true});
 // Cloudflare publishes the permanent validation CNAME; ACM handles renewals.
 const validated=new aws.acm.CertificateValidation('public-tls',{certificateArn:certificate.arn},{customTimeouts:{create:'30m'}});
 const https=new aws.lb.Listener('https',{loadBalancerArn:loadBalancer.arn,port:443,protocol:'HTTPS',certificateArn:c.certificateArn??validated.certificateArn,sslPolicy:'ELBSecurityPolicy-TLS13-1-2-2021-06',defaultActions:[{type:'forward',targetGroupArn:target.arn}],tags:c.tags});
 new aws.lb.Listener('http',{loadBalancerArn:loadBalancer.arn,port:80,protocol:'HTTP',defaultActions:[{type:'redirect',redirect:{port:'443',protocol:'HTTPS',statusCode:'HTTP_301'}}],tags:c.tags});
 const template=new aws.ec2.LaunchTemplate('application-fleet',{imageId:c.ami,instanceType:'t4g.small',iamInstanceProfile:{name:c.profile.name},vpcSecurityGroupIds:[c.appSG.id,fleetSG.id],metadataOptions:{httpTokens:'required',httpEndpoint:'enabled',httpPutResponseHopLimit:2},blockDeviceMappings:[{deviceName:'/dev/xvda',ebs:{encrypted:'true',volumeType:'gp3',volumeSize:20,deleteOnTermination:'true'}}],userData:pulumi.all([c.db.address,c.dbSecret.arn,c.ownerSecret.arn,c.repository.repositoryUrl,c.logs.name]).apply(([dbHost,dbArn,ownerArn,repo,logGroup])=>Buffer.from(bootstrap({canonicalHostname:c.canonicalHostname,legacyOrigin:c.legacyOrigin,openrouterSecretArn:c.openrouterSecretArn,hostname:c.hostname,region:c.region,dbHost,dbArn,ownerArn,image:`${repo}@${c.imageDigest}`,caddyImage:c.caddyImage,logGroup,stack:c.stack,behindLoadBalancer:true})).toString('base64')),tags:c.tags});
 const group=new aws.autoscaling.Group('application-fleet',{minSize:2,maxSize:4,desiredCapacity:2,vpcZoneIdentifiers:[c.publicSubnet.id,subnet.id],targetGroupArns:[target.arn],healthCheckType:'ELB',healthCheckGracePeriod:300,defaultInstanceWarmup:120,launchTemplate:{id:template.id,version:template.latestVersion.apply(String)},instanceRefresh:{strategy:'Rolling',preferences:{minHealthyPercentage:100,maxHealthyPercentage:150,instanceWarmup:'120',autoRollback:true,skipMatching:true}},instanceMaintenancePolicy:{minHealthyPercentage:100,maxHealthyPercentage:150},waitForElbCapacity:2,waitForCapacityTimeout:'20m',tags:Object.entries(c.tags).map(([key,value])=>({key,value,propagateAtLaunch:true}))},{dependsOn:[...c.dependencies,route,https],ignoreChanges:['desiredCapacity']});
 new aws.autoscaling.Policy('fleet-cpu',{autoscalingGroupName:group.name,policyType:'TargetTrackingScaling',targetTrackingConfiguration:{targetValue:60,predefinedMetricSpecification:{predefinedMetricType:'ASGAverageCPUUtilization'}}});
 new aws.cloudwatch.MetricAlarm('fleet-unhealthy',{namespace:'AWS/ApplicationELB',metricName:'HealthyHostCount',dimensions:{LoadBalancer:loadBalancer.arnSuffix,TargetGroup:target.arnSuffix},statistic:'Minimum',period:60,evaluationPeriods:2,threshold:2,comparisonOperator:'LessThanThreshold',treatMissingData:'breaching',alarmActions:[c.alerts.arn],okActions:[c.alerts.arn],tags:c.tags});
 return {loadBalancer,certificate,group};
}
