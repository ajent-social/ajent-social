package main

import (
	"encoding/base64"
	"strconv"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/acm"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/lb"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type rollingOutputs struct {
	LoadBalancer *lb.LoadBalancer
	Certificate  *acm.Certificate
	Group        *autoscaling.Group
}

func rollingService(ctx *pulumi.Context, c foundation, zone string) (*rollingOutputs, error) {
	subnet, err := ec2.NewSubnet(ctx, "application-subnet-1", &ec2.SubnetArgs{VpcId: c.VPC.ID(), CidrBlock: pulumi.String("10.87.2.0/24"), AvailabilityZone: pulumi.String(zone), MapPublicIpOnLaunch: pulumi.Bool(true), Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	route, err := ec2.NewRouteTableAssociation(ctx, "application-route-1", &ec2.RouteTableAssociationArgs{SubnetId: subnet.ID(), RouteTableId: c.Route.ID()})
	if err != nil {
		return nil, err
	}
	edgeIngress := ec2.SecurityGroupIngressArray{}
	for _, port := range []int{80, 443} {
		edgeIngress = append(edgeIngress, &ec2.SecurityGroupIngressArgs{Protocol: pulumi.String("tcp"), FromPort: pulumi.Int(port), ToPort: pulumi.Int(port), CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")}})
	}
	edgeSG, err := ec2.NewSecurityGroup(ctx, "edge", &ec2.SecurityGroupArgs{VpcId: c.VPC.ID(), Ingress: edgeIngress, Egress: ec2.SecurityGroupEgressArray{&ec2.SecurityGroupEgressArgs{Protocol: pulumi.String("tcp"), FromPort: pulumi.Int(8080), ToPort: pulumi.Int(8080), CidrBlocks: pulumi.StringArray{pulumi.String("10.87.1.0/24"), pulumi.String("10.87.2.0/24")}}}, Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	fleetSG, err := ec2.NewSecurityGroup(ctx, "fleet", &ec2.SecurityGroupArgs{VpcId: c.VPC.ID(), Ingress: ec2.SecurityGroupIngressArray{&ec2.SecurityGroupIngressArgs{Protocol: pulumi.String("tcp"), FromPort: pulumi.Int(8080), ToPort: pulumi.Int(8080), SecurityGroups: pulumi.StringArray{edgeSG.ID()}}}, Egress: ec2.SecurityGroupEgressArray{}, Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	balancer, err := lb.NewLoadBalancer(ctx, "public-edge", &lb.LoadBalancerArgs{LoadBalancerType: pulumi.String("application"), Internal: pulumi.Bool(false), Subnets: pulumi.StringArray{c.PublicSubnet.ID(), subnet.ID()}, SecurityGroups: pulumi.StringArray{edgeSG.ID()}, DropInvalidHeaderFields: pulumi.Bool(true), EnableDeletionProtection: pulumi.Bool(true), Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	target, err := lb.NewTargetGroup(ctx, "application-fleet", &lb.TargetGroupArgs{VpcId: c.VPC.ID(), Port: pulumi.Int(8080), Protocol: pulumi.String("HTTP"), TargetType: pulumi.String("instance"), DeregistrationDelay: pulumi.Int(30), HealthCheck: &lb.TargetGroupHealthCheckArgs{Path: pulumi.String("/healthz"), Port: pulumi.String("traffic-port"), Protocol: pulumi.String("HTTP"), Matcher: pulumi.String("200"), Interval: pulumi.Int(15), Timeout: pulumi.Int(5), HealthyThreshold: pulumi.Int(2), UnhealthyThreshold: pulumi.Int(2)}, Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	// Preserve the original protected certificate while the apex certificate is in use.
	certificateDomain := c.Settings.Hostname
	sans := pulumi.StringArray{}
	if c.Settings.CertificateARN != "" {
		certificateDomain = "dogfood.ajent.social"
	} else if c.Settings.CanonicalHostname == "ajent.social" {
		sans = append(sans, pulumi.String("www.ajent.social"))
	}
	cert, err := acm.NewCertificate(ctx, "public-tls", &acm.CertificateArgs{DomainName: pulumi.String(certificateDomain), SubjectAlternativeNames: sans, ValidationMethod: pulumi.String("DNS"), Tags: c.Tags}, pulumi.Protect(true))
	if err != nil {
		return nil, err
	}
	validated, err := acm.NewCertificateValidation(ctx, "public-tls", &acm.CertificateValidationArgs{CertificateArn: cert.Arn}, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "30m"}))
	if err != nil {
		return nil, err
	}
	var listenerCertificate pulumi.StringInput = validated.CertificateArn
	if c.Settings.CertificateARN != "" {
		listenerCertificate = pulumi.String(c.Settings.CertificateARN)
	}
	https, err := lb.NewListener(ctx, "https", &lb.ListenerArgs{LoadBalancerArn: balancer.Arn, Port: pulumi.Int(443), Protocol: pulumi.String("HTTPS"), CertificateArn: listenerCertificate, SslPolicy: pulumi.String("ELBSecurityPolicy-TLS13-1-2-2021-06"), DefaultActions: lb.ListenerDefaultActionArray{&lb.ListenerDefaultActionArgs{Type: pulumi.String("forward"), TargetGroupArn: target.Arn}}, Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	if _, err = lb.NewListener(ctx, "http", &lb.ListenerArgs{LoadBalancerArn: balancer.Arn, Port: pulumi.Int(80), Protocol: pulumi.String("HTTP"), DefaultActions: lb.ListenerDefaultActionArray{&lb.ListenerDefaultActionArgs{Type: pulumi.String("redirect"), Redirect: &lb.ListenerDefaultActionRedirectArgs{Port: pulumi.String("443"), Protocol: pulumi.String("HTTPS"), StatusCode: pulumi.String("HTTP_301")}}}, Tags: c.Tags}); err != nil {
		return nil, err
	}
	template, err := ec2.NewLaunchTemplate(ctx, "application-fleet", &ec2.LaunchTemplateArgs{ImageId: c.AMI, InstanceType: pulumi.String("t4g.small"), IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileArgs{Name: c.Profile.Name}, VpcSecurityGroupIds: pulumi.StringArray{c.AppSG.ID(), fleetSG.ID()}, MetadataOptions: &ec2.LaunchTemplateMetadataOptionsArgs{HttpTokens: pulumi.String("required"), HttpEndpoint: pulumi.String("enabled"), HttpPutResponseHopLimit: pulumi.Int(2)}, BlockDeviceMappings: ec2.LaunchTemplateBlockDeviceMappingArray{&ec2.LaunchTemplateBlockDeviceMappingArgs{DeviceName: pulumi.String("/dev/xvda"), Ebs: &ec2.LaunchTemplateBlockDeviceMappingEbsArgs{Encrypted: pulumi.String("true"), VolumeType: pulumi.String("gp3"), VolumeSize: pulumi.Int(20), DeleteOnTermination: pulumi.String("true")}}}, UserData: c.userData(true).ApplyT(func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }).(pulumi.StringOutput), Tags: c.Tags})
	if err != nil {
		return nil, err
	}
	// Preserve the TypeScript tag-array order; Go map iteration is nondeterministic.
	groupTags := autoscaling.GroupTagArray{}
	for _, key := range []string{"Project", "Environment", "ManagedBy"} {
		groupTags = append(groupTags, &autoscaling.GroupTagArgs{Key: pulumi.String(key), Value: c.Tags[key], PropagateAtLaunch: pulumi.Bool(true)})
	}
	deps := append(append([]pulumi.Resource{}, c.Dependencies...), route, https)
	group, err := autoscaling.NewGroup(ctx, "application-fleet", &autoscaling.GroupArgs{MinSize: pulumi.Int(2), MaxSize: pulumi.Int(4), DesiredCapacity: pulumi.Int(2), VpcZoneIdentifiers: pulumi.StringArray{c.PublicSubnet.ID(), subnet.ID()}, TargetGroupArns: pulumi.StringArray{target.Arn}, HealthCheckType: pulumi.String("ELB"), HealthCheckGracePeriod: pulumi.Int(300), DefaultInstanceWarmup: pulumi.Int(120), LaunchTemplate: &autoscaling.GroupLaunchTemplateArgs{Id: template.ID(), Version: template.LatestVersion.ApplyT(strconv.Itoa).(pulumi.StringOutput)}, InstanceRefresh: &autoscaling.GroupInstanceRefreshArgs{Strategy: pulumi.String("Rolling"), Preferences: &autoscaling.GroupInstanceRefreshPreferencesArgs{MinHealthyPercentage: pulumi.Int(100), MaxHealthyPercentage: pulumi.Int(150), InstanceWarmup: pulumi.String("120"), AutoRollback: pulumi.Bool(true), SkipMatching: pulumi.Bool(true)}}, InstanceMaintenancePolicy: &autoscaling.GroupInstanceMaintenancePolicyArgs{MinHealthyPercentage: pulumi.Int(100), MaxHealthyPercentage: pulumi.Int(150)}, WaitForElbCapacity: pulumi.Int(2), WaitForCapacityTimeout: pulumi.String("20m"), Tags: groupTags}, pulumi.DependsOn(deps), pulumi.IgnoreChanges([]string{"desiredCapacity"}))
	if err != nil {
		return nil, err
	}
	if _, err = autoscaling.NewPolicy(ctx, "fleet-cpu", &autoscaling.PolicyArgs{AutoscalingGroupName: group.Name, PolicyType: pulumi.String("TargetTrackingScaling"), TargetTrackingConfiguration: &autoscaling.PolicyTargetTrackingConfigurationArgs{TargetValue: pulumi.Float64(60), PredefinedMetricSpecification: &autoscaling.PolicyTargetTrackingConfigurationPredefinedMetricSpecificationArgs{PredefinedMetricType: pulumi.String("ASGAverageCPUUtilization")}}}); err != nil {
		return nil, err
	}
	if _, err = cloudwatch.NewMetricAlarm(ctx, "fleet-unhealthy", &cloudwatch.MetricAlarmArgs{Namespace: pulumi.String("AWS/ApplicationELB"), MetricName: pulumi.String("HealthyHostCount"), Dimensions: pulumi.StringMap{"LoadBalancer": balancer.ArnSuffix, "TargetGroup": target.ArnSuffix}, Statistic: pulumi.String("Minimum"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(2), Threshold: pulumi.Float64(2), ComparisonOperator: pulumi.String("LessThanThreshold"), TreatMissingData: pulumi.String("breaching"), AlarmActions: pulumi.Array{c.Alerts.Arn}, OkActions: pulumi.Array{c.Alerts.Arn}, Tags: c.Tags}); err != nil {
		return nil, err
	}
	return &rollingOutputs{LoadBalancer: balancer, Certificate: cert, Group: group}, nil
}
