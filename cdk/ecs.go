package main

import (
	"fmt"
	"log"
	"path"
	"strconv"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsefs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type ECSResourcesProps struct {
	Vpc                   awsec2.Vpc
	SecurityGroup         awsec2.SecurityGroup
	ServerSubDomain       string
	Domain                string
	HostedZoneId          string
	MemorySize            string
	CpuSize               string
	SnsTopic              awssns.Topic
	StartupMin            string
	ShutdownMin           string
	ServerImage           string
	ServerPort            int
	ServerProtocol        awsecs.Protocol
	ServerDebug           bool
	SubDomainHostedZoneId string
	EnablePersistence     bool
	MinecraftServerConfig ServerConfig
}

type ECSResources struct {
	Task    awsecs.FargateTaskDefinition
	Cluster awsecs.Cluster
	Service awsecs.FargateService
}

func NewECSResources(scope constructs.Construct, id string, props *ECSResourcesProps) ECSResources {
	// Create ECS Cluster
	clusterID := fmt.Sprintf("%s-Cluster", id)
	cluster := awsecs.NewCluster(scope, jsii.String(clusterID), &awsecs.ClusterProps{
		Vpc:                            props.Vpc,
		ClusterName:                    jsii.String(clusterID),
		ContainerInsights:              jsii.Bool(true),
		EnableFargateCapacityProviders: jsii.Bool(true),
	})

	// Declare variables for EFS resources
	var fileSystem awsefs.FileSystem
	var accessPoint awsefs.AccessPoint

	if props.EnablePersistence {
		// Create EFS File System if persistence is enabled
		fsID := fmt.Sprintf("%s-FileSystem", id)
		fileSystem = awsefs.NewFileSystem(scope, jsii.String(fsID), &awsefs.FileSystemProps{
			Vpc:            props.Vpc,
			FileSystemName: jsii.String(fsID),
			RemovalPolicy:  awscdk.RemovalPolicy_RETAIN,
		})

		// EFS Access Point
		accessPointID := fmt.Sprintf("%s-AccessPoint", id)
		accessPoint = awsefs.NewAccessPoint(scope, jsii.String(accessPointID), &awsefs.AccessPointProps{
			FileSystem: fileSystem,
			Path:       jsii.String("/minecraft"),
			PosixUser: &awsefs.PosixUser{
				Uid: jsii.String("1000"),
				Gid: jsii.String("1000"),
			},
			CreateAcl: &awsefs.Acl{
				OwnerGid:    jsii.String("1000"),
				OwnerUid:    jsii.String("1000"),
				Permissions: jsii.String("0750"),
			},
		})
	}

	// IAM Role for ECS Tasks
	taskRoleID := fmt.Sprintf("%s-TaskRole", id)
	taskRole := awsiam.NewRole(scope, jsii.String(taskRoleID), &awsiam.RoleProps{
		RoleName:  jsii.String(taskRoleID),
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("ecs-tasks.amazonaws.com"), nil),
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"TaskPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: jsii.Strings(
							"elasticfilesystem:ClientMount",
							"elasticfilesystem:ClientWrite",
							"elasticfilesystem:DescribeFileSystems",
							"ecs:DescribeTasks",
						),
						Resources: jsii.Strings("*"),
					}),
				},
			}),
		},
	})

	// Fargate Task Definition
	taskDefID := fmt.Sprintf("%s-TaskDefinition", id)
	task := awsecs.NewFargateTaskDefinition(scope, jsii.String(taskDefID), &awsecs.FargateTaskDefinitionProps{
		MemoryLimitMiB: jsii.Number(parseFloat(props.MemorySize)),
		Cpu:            jsii.Number(parseFloat(props.CpuSize)),
		RuntimePlatform: &awsecs.RuntimePlatform{
			OperatingSystemFamily: awsecs.OperatingSystemFamily_LINUX(),
			CpuArchitecture:       awsecs.CpuArchitecture_ARM64(),
		},
		TaskRole: taskRole,
	})

	// Fargate Service
	serviceID := fmt.Sprintf("%s-FargateService", id)
	service := awsecs.NewFargateService(scope, jsii.String(serviceID), &awsecs.FargateServiceProps{
		ServiceName: jsii.String(serviceID),
		Cluster:     cluster,
		CapacityProviderStrategies: &[]*awsecs.CapacityProviderStrategy{
			{
				CapacityProvider: jsii.String("FARGATE"),
				Weight:           jsii.Number(1),
				Base:             jsii.Number(1),
			},
		},
		TaskDefinition:       task,
		AssignPublicIp:       jsii.Bool(true),
		DesiredCount:         jsii.Number(0),
		VpcSubnets:           &awsec2.SubnetSelection{SubnetType: awsec2.SubnetType_PUBLIC},
		SecurityGroups:       &[]awsec2.ISecurityGroup{props.SecurityGroup},
		EnableExecuteCommand: jsii.Bool(true),
	})

	var loggingDriver awsecs.LogDriver
	if props.ServerDebug {
		logPrefix := fmt.Sprintf("%s-Log", id)
		loggingDriver = awsecs.NewAwsLogDriver(&awsecs.AwsLogDriverProps{
			LogRetention: awslogs.RetentionDays_THREE_DAYS,
			StreamPrefix: jsii.String(logPrefix),
		})
	} else {
		loggingDriver = nil
	}

	// Main Server Container Definition
	containerID := fmt.Sprintf("%s-ServerContainer", id)
	serverContainer := task.AddContainer(jsii.String(containerID), &awsecs.ContainerDefinitionOptions{
		Image: awsecs.ContainerImage_FromRegistry(jsii.String(props.ServerImage), nil),
		Environment: &map[string]*string{
			"EULA":                         jsii.String("TRUE"),
			"MEMORY":                       jsii.String("8G"),
			"VERSION":                      jsii.String(props.MinecraftServerConfig.Version),
			"MOTD":                         jsii.String(props.MinecraftServerConfig.Motd),
			"DIFFICULTY":                   jsii.String(props.MinecraftServerConfig.Difficulty),
			"MAX_PLAYERS":                  jsii.String(props.MinecraftServerConfig.MaxPlayers),
			"ALLOW_NETHER":                 jsii.String(props.MinecraftServerConfig.AllowNether),
			"ANNOUNCE_PLAYER_ACHIEVEMENTS": jsii.String(props.MinecraftServerConfig.AnnouncePlayerAchievements),
			"GENERATE_STRUCTURES":          jsii.String(props.MinecraftServerConfig.GenerateStructures),
			"HARDCORE":                     jsii.String(props.MinecraftServerConfig.Hardcore),
			"SNOOPER_ENABLED":              jsii.String(props.MinecraftServerConfig.SnooperEnabled),
			"MAX_BUILD_HEIGHT":             jsii.String(props.MinecraftServerConfig.MaxBuildHeight),
			"SPAWN_ANIMALS":                jsii.String(props.MinecraftServerConfig.SpawnAnimals),
			"SPAWN_MONSTERS":               jsii.String(props.MinecraftServerConfig.SpawnMonsters),
			"SPAWN_NPCS":                   jsii.String(props.MinecraftServerConfig.SpawnNpcs),
			"SEED":                         jsii.String(props.MinecraftServerConfig.Seed),
			"MODE":                         jsii.String(props.MinecraftServerConfig.Mode),
			"PVP":                          jsii.String(props.MinecraftServerConfig.Pvp),
			"ONLINE_MODE":                  jsii.String(props.MinecraftServerConfig.OnlineMode),
			"SERVER_NAME":                  jsii.String(props.MinecraftServerConfig.ServerName),
			"ENABLE_WHITELIST":             jsii.String(props.MinecraftServerConfig.EnableWhitelist),
			"WHITELIST":                    jsii.String(props.MinecraftServerConfig.Whitelist),
			"OP_PERMISSION_LEVEL":          jsii.String(props.MinecraftServerConfig.OpPermissionLevel),
		},
		PortMappings: &[]*awsecs.PortMapping{
			{
				ContainerPort: jsii.Number(props.ServerPort),
				HostPort:      jsii.Number(props.ServerPort),
				Protocol:      props.ServerProtocol,
			},
		},
		Logging: loggingDriver,
	})

	if props.EnablePersistence {
		volumeID := fmt.Sprintf("%s-DataVolume", id)
		task.AddVolume(&awsecs.Volume{
			Name: jsii.String(volumeID),
			EfsVolumeConfiguration: &awsecs.EfsVolumeConfiguration{
				FileSystemId:      fileSystem.FileSystemId(),
				TransitEncryption: jsii.String("ENABLED"),
				AuthorizationConfig: &awsecs.AuthorizationConfig{
					AccessPointId: accessPoint.AccessPointId(),
					Iam:           jsii.String("ENABLED"),
				},
			},
		})

		serverContainer.AddMountPoints(&awsecs.MountPoint{
			ContainerPath: jsii.String("/data"),
			SourceVolume:  jsii.String(volumeID),
			ReadOnly:      jsii.Bool(false),
		})

		// Connect FileSystem to Service
		fileSystem.Connections().AllowDefaultPortFrom(service, jsii.String("Allow ECS service to access EFS"))
	}

	// Add Watchdog Container
	watchdogContainerID := fmt.Sprintf("%s-WatchdogContainer", id)
	task.AddContainer(jsii.String(watchdogContainerID), &awsecs.ContainerDefinitionOptions{
		Image: awsecs.ContainerImage_FromAsset(jsii.String(path.Join(".", "cmd", "watchdog")), &awsecs.AssetImageProps{
			File: jsii.String("Dockerfile"),
		}),
		Essential: jsii.Bool(true),
		Environment: &map[string]*string{
			"CLUSTER":     cluster.ClusterName(),
			"SERVICE":     jsii.String(serviceID),
			"DNSZONE":     jsii.String(props.SubDomainHostedZoneId),
			"SERVERNAME":  jsii.String(fmt.Sprintf("%s.%s", props.ServerSubDomain, props.Domain)),
			"SNSTOPIC":    props.SnsTopic.TopicArn(),
			"STARTUPMIN":  jsii.String(props.StartupMin),
			"SHUTDOWNMIN": jsii.String(props.ShutdownMin),
		},
		Logging: loggingDriver,
	})

	// IAM Policies for Watchdog
	policyID := fmt.Sprintf("%s-ServerPolicy", id)
	serverPolicy := awsiam.NewPolicy(scope, jsii.String(policyID), &awsiam.PolicyProps{
		PolicyName: jsii.String(policyID),
		Statements: &[]awsiam.PolicyStatement{
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Actions: jsii.Strings("ecs:*"),
				Resources: jsii.Strings(
					*task.TaskDefinitionArn(),
					fmt.Sprintf("%s/*", *task.TaskDefinitionArn()),
					*service.ServiceArn(),
					fmt.Sprintf("%s/*", *service.ServiceArn()),
					*cluster.ClusterArn(),
					fmt.Sprintf("%s/*", *cluster.ClusterArn()),
				),
			}),
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Actions:   jsii.Strings("ec2:DescribeNetworkInterfaces"),
				Resources: jsii.Strings("*"),
			}),
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Actions:   jsii.Strings("sns:Publish"),
				Resources: &[]*string{props.SnsTopic.TopicArn()},
			}),
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Actions:   jsii.Strings("route53:GetHostedZone", "route53:ChangeResourceRecordSets", "route53:ListResourceRecordSets"),
				Resources: jsii.Strings(fmt.Sprintf("arn:aws:route53:::hostedzone/%s", props.SubDomainHostedZoneId)),
			}),
		},
	})
	serverPolicy.AttachToRole(taskRole)

	return ECSResources{
		Task:    task,
		Cluster: cluster,
		Service: service,
	}
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Fatalf("Invalid float value: %s", value)
	}
	return parsed
}
