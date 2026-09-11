package main

import (
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ecr"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/rds"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/sns"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ssm"
	"github.com/pulumi/pulumi-cloudflare/sdk/v6/go/cloudflare"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		s, err := loadSettings(ctx)
		if err != nil {
			return err
		}
		return deploy(ctx, s)
	})
}

type foundation struct {
	Settings              settings
	Tags                  pulumi.StringMap
	VPC                   *ec2.Vpc
	PublicSubnet          *ec2.Subnet
	Route                 *ec2.RouteTable
	AppSG                 *ec2.SecurityGroup
	Profile               *iam.InstanceProfile
	AMI                   pulumi.StringInput
	Repository            *ecr.Repository
	Logs                  *cloudwatch.LogGroup
	DB                    *rds.Instance
	DBSecret, OwnerSecret *secretsmanager.Secret
	Alerts                *sns.Topic
	Dependencies          []pulumi.Resource
}

func deploy(ctx *pulumi.Context, s settings) error {
	if err := s.validate(); err != nil {
		return err
	}
	tags := pulumi.StringMap{"Project": pulumi.String("ajent"), "Environment": pulumi.String(s.Stack), "ManagedBy": pulumi.String("Pulumi")}
	az, err := aws.GetAvailabilityZones(ctx, &aws.GetAvailabilityZonesArgs{State: pulumi.StringRef("available")})
	if err != nil {
		return fmt.Errorf("looking up availability zones: %w", err)
	}
	if len(az.Names) < 2 {
		return fmt.Errorf("two AZs required")
	}
	vpc, err := ec2.NewVpc(ctx, "network", &ec2.VpcArgs{CidrBlock: pulumi.String("10.87.0.0/16"), EnableDnsHostnames: pulumi.Bool(true), EnableDnsSupport: pulumi.Bool(true), Tags: tags})
	if err != nil {
		return err
	}
	gateway, err := ec2.NewInternetGateway(ctx, "internet", &ec2.InternetGatewayArgs{VpcId: vpc.ID(), Tags: tags})
	if err != nil {
		return err
	}
	publicSubnet, err := ec2.NewSubnet(ctx, "application-subnet", &ec2.SubnetArgs{VpcId: vpc.ID(), CidrBlock: pulumi.String("10.87.1.0/24"), AvailabilityZone: pulumi.String(az.Names[0]), MapPublicIpOnLaunch: pulumi.Bool(true), Tags: tags})
	if err != nil {
		return err
	}
	privateIDs := pulumi.StringArray{}
	for i := 0; i < 2; i++ {
		subnet, err := ec2.NewSubnet(ctx, fmt.Sprintf("database-subnet-%d", i), &ec2.SubnetArgs{VpcId: vpc.ID(), CidrBlock: pulumi.String(fmt.Sprintf("10.87.%d.0/24", 10+i)), AvailabilityZone: pulumi.String(az.Names[i]), Tags: tags})
		if err != nil {
			return err
		}
		privateIDs = append(privateIDs, subnet.ID())
	}
	route, err := ec2.NewRouteTable(ctx, "public-routes", &ec2.RouteTableArgs{VpcId: vpc.ID(), Routes: ec2.RouteTableRouteArray{&ec2.RouteTableRouteArgs{CidrBlock: pulumi.String("0.0.0.0/0"), GatewayId: gateway.ID()}}, Tags: tags})
	if err != nil {
		return err
	}
	if _, err = ec2.NewRouteTableAssociation(ctx, "application-route", &ec2.RouteTableAssociationArgs{SubnetId: publicSubnet.ID(), RouteTableId: route.ID()}); err != nil {
		return err
	}
	ingress := ec2.SecurityGroupIngressArray{}
	if !s.RetireLegacyHost {
		for _, port := range []int{80, 443} {
			ingress = append(ingress, &ec2.SecurityGroupIngressArgs{Protocol: pulumi.String("tcp"), FromPort: pulumi.Int(port), ToPort: pulumi.Int(port), CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")}})
		}
	}
	appSG, err := ec2.NewSecurityGroup(ctx, "application", &ec2.SecurityGroupArgs{VpcId: vpc.ID(), Ingress: ingress, Egress: ec2.SecurityGroupEgressArray{&ec2.SecurityGroupEgressArgs{Protocol: pulumi.String("-1"), FromPort: pulumi.Int(0), ToPort: pulumi.Int(0), CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")}}}, Tags: tags})
	if err != nil {
		return err
	}
	dbSG, err := ec2.NewSecurityGroup(ctx, "database", &ec2.SecurityGroupArgs{VpcId: vpc.ID(), Ingress: ec2.SecurityGroupIngressArray{&ec2.SecurityGroupIngressArgs{Protocol: pulumi.String("tcp"), FromPort: pulumi.Int(5432), ToPort: pulumi.Int(5432), SecurityGroups: pulumi.StringArray{appSG.ID()}}}, Tags: tags})
	if err != nil {
		return err
	}
	subnetGroup, err := rds.NewSubnetGroup(ctx, "database", &rds.SubnetGroupArgs{SubnetIds: privateIDs, Tags: tags})
	if err != nil {
		return err
	}
	dbPassword, err := random.NewRandomPassword(ctx, "database-password", &random.RandomPasswordArgs{Length: pulumi.Int(48), Special: pulumi.Bool(false)})
	if err != nil {
		return err
	}
	ownerPassword, err := random.NewRandomPassword(ctx, "owner-key", &random.RandomPasswordArgs{Length: pulumi.Int(64), Special: pulumi.Bool(false)})
	if err != nil {
		return err
	}
	dbSecret, err := secretsmanager.NewSecret(ctx, "database-secret", &secretsmanager.SecretArgs{RecoveryWindowInDays: pulumi.Int(7), Tags: tags}, pulumi.Protect(true))
	if err != nil {
		return err
	}
	ownerSecret, err := secretsmanager.NewSecret(ctx, "owner-secret", &secretsmanager.SecretArgs{RecoveryWindowInDays: pulumi.Int(7), Tags: tags}, pulumi.Protect(true))
	if err != nil {
		return err
	}
	dbVersion, err := secretsmanager.NewSecretVersion(ctx, "database-secret-value", &secretsmanager.SecretVersionArgs{SecretId: dbSecret.ID(), SecretString: dbPassword.Result})
	if err != nil {
		return err
	}
	ownerVersion, err := secretsmanager.NewSecretVersion(ctx, "owner-secret-value", &secretsmanager.SecretVersionArgs{SecretId: ownerSecret.ID(), SecretString: ownerPassword.Result})
	if err != nil {
		return err
	}
	db, err := rds.NewInstance(ctx, "database", &rds.InstanceArgs{
		Engine: pulumi.String("postgres"), EngineVersion: pulumi.String("16"), InstanceClass: pulumi.String("db.t4g.micro"), AllocatedStorage: pulumi.Int(20), MaxAllocatedStorage: pulumi.Int(100), StorageType: pulumi.String("gp3"), StorageEncrypted: pulumi.Bool(true),
		DbName: pulumi.String("ajent"), Username: pulumi.String("ajent"), Password: dbPassword.Result, Port: pulumi.Int(5432), DbSubnetGroupName: subnetGroup.Name, VpcSecurityGroupIds: pulumi.StringArray{dbSG.ID()},
		PubliclyAccessible: pulumi.Bool(false), MultiAz: pulumi.Bool(false), BackupRetentionPeriod: pulumi.Int(7), BackupWindow: pulumi.String("09:00-10:00"), MaintenanceWindow: pulumi.String("sun:10:00-sun:11:00"),
		DeletionProtection: pulumi.Bool(true), SkipFinalSnapshot: pulumi.Bool(false), FinalSnapshotIdentifier: pulumi.String("ajent-" + s.Stack + "-final"), AutoMinorVersionUpgrade: pulumi.Bool(true), CopyTagsToSnapshot: pulumi.Bool(true), Tags: tags,
	}, pulumi.Protect(true))
	if err != nil {
		return err
	}
	repository, err := ecr.NewRepository(ctx, "application-images", &ecr.RepositoryArgs{ImageTagMutability: pulumi.String("IMMUTABLE"), ImageScanningConfiguration: &ecr.RepositoryImageScanningConfigurationArgs{ScanOnPush: pulumi.Bool(true)}, ForceDelete: pulumi.Bool(false), Tags: tags})
	if err != nil {
		return err
	}
	logs, err := cloudwatch.NewLogGroup(ctx, "application-logs", &cloudwatch.LogGroupArgs{RetentionInDays: pulumi.Int(30), Tags: tags})
	if err != nil {
		return err
	}
	alerts, err := sns.NewTopic(ctx, "alerts", &sns.TopicArgs{Tags: tags})
	if err != nil {
		return err
	}
	if s.AlertEmail != "" {
		if _, err = sns.NewTopicSubscription(ctx, "operator", &sns.TopicSubscriptionArgs{Topic: alerts.Arn, Protocol: pulumi.String("email"), Endpoint: pulumi.String(s.AlertEmail)}); err != nil {
			return err
		}
	}
	for _, a := range []struct {
		name, metric, comparison string
		threshold                float64
	}{{"database-storage", "FreeStorageSpace", "LessThanThreshold", 5 * 1024 * 1024 * 1024}, {"database-cpu", "CPUUtilization", "GreaterThanThreshold", 85}} {
		if _, err = cloudwatch.NewMetricAlarm(ctx, a.name, &cloudwatch.MetricAlarmArgs{Namespace: pulumi.String("AWS/RDS"), MetricName: pulumi.String(a.metric), Dimensions: pulumi.StringMap{"DBInstanceIdentifier": db.Identifier}, Statistic: pulumi.String("Average"), Period: pulumi.Int(300), EvaluationPeriods: pulumi.Int(3), Threshold: pulumi.Float64(a.threshold), ComparisonOperator: pulumi.String(a.comparison), TreatMissingData: pulumi.String("breaching"), AlarmActions: pulumi.Array{alerts.Arn}, OkActions: pulumi.Array{alerts.Arn}, Tags: tags}); err != nil {
			return err
		}
	}
	// No default metric value: missing worker events must remain missing even when
	// unrelated application traffic continues. Minimum catches any degraded replica.
	if s.ImprovementRepository != "" {
		metricName := "ImprovementWorkerHealthy-" + s.Stack
		heartbeat, err := cloudwatch.NewLogMetricFilter(ctx, "improvement-worker-health", &cloudwatch.LogMetricFilterArgs{
			LogGroupName: logs.Name,
			Pattern:      pulumi.String(`{ $.event = "improvement_worker" && ($.healthy = 0 || $.healthy = 1) }`),
			MetricTransformation: &cloudwatch.LogMetricFilterMetricTransformationArgs{Name: pulumi.String(metricName), Namespace: pulumi.String("Ajent/Production"), Value: pulumi.String("$.healthy")},
		})
		if err != nil {
			return err
		}
		if _, err = cloudwatch.NewMetricAlarm(ctx, "improvement-worker-stalled", &cloudwatch.MetricAlarmArgs{
			Namespace: pulumi.String("Ajent/Production"), MetricName: pulumi.String(metricName), Statistic: pulumi.String("Minimum"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(5), DatapointsToAlarm: pulumi.Int(5), Threshold: pulumi.Float64(1), ComparisonOperator: pulumi.String("LessThanThreshold"), TreatMissingData: pulumi.String("breaching"), AlarmActions: pulumi.Array{alerts.Arn}, OkActions: pulumi.Array{alerts.Arn}, Tags: tags,
		}, pulumi.DependsOn([]pulumi.Resource{heartbeat})); err != nil {
			return err
		}
		dueMetric := "ImprovementWorkerOldestDueSeconds-" + s.Stack
		due, err := cloudwatch.NewLogMetricFilter(ctx, "improvement-worker-oldest-due", &cloudwatch.LogMetricFilterArgs{
			LogGroupName: logs.Name,
			Pattern:      pulumi.String(`{ $.event = "improvement_worker" && $.oldest_due_seconds >= 0 }`),
			MetricTransformation: &cloudwatch.LogMetricFilterMetricTransformationArgs{Name: pulumi.String(dueMetric), Namespace: pulumi.String("Ajent/Production"), Value: pulumi.String("$.oldest_due_seconds")},
		})
		if err != nil {
			return err
		}
		if _, err = cloudwatch.NewMetricAlarm(ctx, "improvement-worker-overdue", &cloudwatch.MetricAlarmArgs{
			Namespace: pulumi.String("Ajent/Production"), MetricName: pulumi.String(dueMetric), Statistic: pulumi.String("Maximum"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(1), Threshold: pulumi.Float64(900), ComparisonOperator: pulumi.String("GreaterThanOrEqualToThreshold"), TreatMissingData: pulumi.String("notBreaching"), AlarmActions: pulumi.Array{alerts.Arn}, OkActions: pulumi.Array{alerts.Arn}, Tags: tags,
		}, pulumi.DependsOn([]pulumi.Resource{due})); err != nil {
			return err
		}
	}
	var edge *rollingOutputs
	var host *ec2.Instance
	var address *ec2.Eip
	if s.ImageDigest != "" {
		role, err := iam.NewRole(ctx, "host", &iam.RoleArgs{AssumeRolePolicy: pulumi.String(`{"Version":"2012-10-17","Statement":[{"Action":"sts:AssumeRole","Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Sid":"AllowAssumeRole"}]}`), Tags: tags})
		if err != nil {
			return err
		}
		ssmAttachment, err := iam.NewRolePolicyAttachment(ctx, "ssm", &iam.RolePolicyAttachmentArgs{Role: role.Name, PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore")})
		if err != nil {
			return err
		}
		policyJSON := pulumi.All(dbSecret.Arn, ownerSecret.Arn, repository.Arn, logs.Arn).ApplyT(func(v []any) (string, error) {
			secretARNs := []string{v[0].(string), v[1].(string)}
			if s.OpenrouterSecretARN != "" {
				secretARNs = append(secretARNs, s.OpenrouterSecretARN)
			}
			if s.ImprovementGitHubSecretARN != "" {
				secretARNs = append(secretARNs, s.ImprovementGitHubSecretARN)
			}
			policy := map[string]any{"Version": "2012-10-17", "Statement": []any{
				map[string]any{"Effect": "Allow", "Action": []string{"secretsmanager:GetSecretValue"}, "Resource": secretARNs},
				map[string]any{"Effect": "Allow", "Action": []string{"ecr:GetAuthorizationToken"}, "Resource": "*"},
				map[string]any{"Effect": "Allow", "Action": []string{"ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer", "ecr:BatchCheckLayerAvailability"}, "Resource": v[2]},
				map[string]any{"Effect": "Allow", "Action": []string{"logs:CreateLogStream", "logs:PutLogEvents"}, "Resource": v[3].(string) + ":*"},
				map[string]any{"Effect": "Allow", "Action": []string{"cloudwatch:PutMetricData"}, "Resource": "*", "Condition": map[string]any{"StringEquals": map[string]string{"cloudwatch:namespace": "Ajent/Production"}}},
			}}
			b, err := json.Marshal(policy)
			return string(b), err
		}).(pulumi.StringOutput)
		policy, err := iam.NewRolePolicy(ctx, "runtime", &iam.RolePolicyArgs{Role: role.ID(), Policy: policyJSON})
		if err != nil {
			return err
		}
		profile, err := iam.NewInstanceProfile(ctx, "host", &iam.InstanceProfileArgs{Role: role.Name, Tags: tags})
		if err != nil {
			return err
		}
		ami, err := ssm.LookupParameter(ctx, &ssm.LookupParameterArgs{Name: "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"})
		if err != nil {
			return err
		}
		deps := []pulumi.Resource{policy, ssmAttachment, dbVersion, ownerVersion, gateway, route}
		c := foundation{Settings: s, Tags: tags, VPC: vpc, PublicSubnet: publicSubnet, Route: route, AppSG: appSG, Profile: profile, AMI: pulumi.String(ami.Value), Repository: repository, Logs: logs, DB: db, DBSecret: dbSecret, OwnerSecret: ownerSecret, Alerts: alerts, Dependencies: deps}
		if s.RollingService {
			edge, err = rollingService(ctx, c, az.Names[1])
			if err != nil {
				return err
			}
		}
		if !s.RetireLegacyHost {
			host, err = ec2.NewInstance(ctx, "application-host", &ec2.InstanceArgs{Ami: c.AMI, InstanceType: pulumi.String("t4g.small"), SubnetId: publicSubnet.ID(), VpcSecurityGroupIds: pulumi.StringArray{appSG.ID()}, IamInstanceProfile: profile.Name,
				AssociatePublicIpAddress: pulumi.Bool(true), MetadataOptions: &ec2.InstanceMetadataOptionsArgs{HttpTokens: pulumi.String("required"), HttpEndpoint: pulumi.String("enabled"), HttpPutResponseHopLimit: pulumi.Int(2)},
				RootBlockDevice: &ec2.InstanceRootBlockDeviceArgs{Encrypted: pulumi.Bool(true), VolumeType: pulumi.String("gp3"), VolumeSize: pulumi.Int(20)}, UserDataReplaceOnChange: pulumi.Bool(true), UserData: c.userData(false), Tags: tags,
			}, pulumi.DependsOn(deps))
			if err != nil {
				return err
			}
			address, err = ec2.NewEip(ctx, "application-address", &ec2.EipArgs{Domain: pulumi.String("vpc"), Instance: host.ID(), Tags: tags})
			if err != nil {
				return err
			}
			if _, err = cloudwatch.NewMetricAlarm(ctx, "host-status", &cloudwatch.MetricAlarmArgs{Namespace: pulumi.String("AWS/EC2"), MetricName: pulumi.String("StatusCheckFailed"), Dimensions: pulumi.StringMap{"InstanceId": host.ID()}, Statistic: pulumi.String("Maximum"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(2), Threshold: pulumi.Float64(1), ComparisonOperator: pulumi.String("GreaterThanOrEqualToThreshold"), TreatMissingData: pulumi.String("breaching"), AlarmActions: pulumi.Array{alerts.Arn}, Tags: tags}); err != nil {
				return err
			}
		}
		for _, a := range []struct {
			name, metric, comparison, statistic string
			threshold                           float64
		}{{"http-health", "Healthy", "LessThanThreshold", "Minimum", 1}, {"host-disk", "DiskUsedPercent", "GreaterThanThreshold", "Maximum", 85}} {
			if _, err = cloudwatch.NewMetricAlarm(ctx, a.name, &cloudwatch.MetricAlarmArgs{Namespace: pulumi.String("Ajent/Production"), MetricName: pulumi.String(a.metric), Dimensions: pulumi.StringMap{"Stack": pulumi.String(s.Stack)}, Statistic: pulumi.String(a.statistic), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(3), Threshold: pulumi.Float64(a.threshold), ComparisonOperator: pulumi.String(a.comparison), TreatMissingData: pulumi.String("breaching"), AlarmActions: pulumi.Array{alerts.Arn}, OkActions: pulumi.Array{alerts.Arn}, Tags: tags}); err != nil {
				return err
			}
		}
		if s.CloudflareZoneID != "" && (edge != nil || address != nil) {
			var content pulumi.StringInput
			kind := "A"
			if edge != nil {
				content = edge.LoadBalancer.DnsName
				kind = "CNAME"
			} else {
				content = address.PublicIp
			}
			if _, err = cloudflare.NewDnsRecord(ctx, "production-dns", &cloudflare.DnsRecordArgs{ZoneId: pulumi.String(s.CloudflareZoneID), Name: pulumi.String(s.Hostname), Type: pulumi.String(kind), Content: content, Ttl: pulumi.Float64(300), Proxied: pulumi.Bool(false), Comment: pulumi.String("Ajent service; DNS-only")}, pulumi.Aliases([]pulumi.Alias{{Name: pulumi.String("dogfood-dns")}})); err != nil {
				return err
			}
		}
	}
	ctx.Export("serviceUrl", pulumi.String("https://"+s.Hostname))
	ctx.Export("repositoryUrl", repository.RepositoryUrl)
	ctx.Export("databaseIdentifier", db.Identifier)
	ctx.Export("ownerSecretArn", ownerSecret.Arn)
	ctx.Export("alertTopicArn", alerts.Arn)
	ctx.Export("logGroupName", logs.Name)
	stage := "foundation-only"
	if s.ImageDigest != "" {
		stage = "host-configured-not-health-verified"
	}
	if edge != nil {
		stage = "rolling-service-configured-verify-refresh-before-release"
	}
	ctx.Export("deploymentStage", pulumi.String(stage))
	if host != nil {
		ctx.Export("instanceId", host.ID())
	}
	if address != nil {
		ctx.Export("publicIp", address.PublicIp)
	}
	if edge != nil {
		ctx.Export("loadBalancerDns", edge.LoadBalancer.DnsName)
		ctx.Export("certificateValidation", edge.Certificate.DomainValidationOptions)
		ctx.Export("autoscalingGroup", edge.Group.Name)
	}
	return nil
}

func (c foundation) userData(rolling bool) pulumi.StringOutput {
	return pulumi.All(c.DB.Address, c.DBSecret.Arn, c.OwnerSecret.Arn, c.Repository.RepositoryUrl, c.Logs.Name).ApplyT(func(v []any) (string, error) {
		config := hostConfig{OpenrouterSecretARN: c.Settings.OpenrouterSecretARN, ImprovementRepository: c.Settings.ImprovementRepository, ImprovementGitHubSecretARN: c.Settings.ImprovementGitHubSecretARN, Hostname: c.Settings.Hostname, Region: c.Settings.Region, DBHost: v[0].(string), DBARN: v[1].(string), OwnerARN: v[2].(string), Image: v[3].(string) + "@" + c.Settings.ImageDigest, CaddyImage: c.Settings.CaddyImage, LogGroup: v[4].(string), Stack: c.Settings.Stack, BehindLoadBalancer: rolling}
		if rolling {
			config.CanonicalHostname = c.Settings.CanonicalHostname
			if config.CanonicalHostname != "" {
				config.LegacyOrigin = "https://" + c.Settings.Hostname
			}
		}
		return bootstrap(config)
	}).(pulumi.StringOutput)
}
