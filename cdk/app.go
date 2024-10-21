package main

import (
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type MinecraftServerStackProps struct {
	awscdk.StackProps
	UsEastLogGroupArn      string
	EcsCpuSize             string
	EcsDebug               string
	EcsMemorySize          string
	EcsMinecraftEdition    string
	EcsShutdownMin         string
	EcsStartupMin          string
	Route53Domain          string
	Route53HostedZoneId    string
	Route53ServerSubDomain string
	SnsEmail               string
	EcsEnablePersistence   bool

	// Server configuration
	MinecraftServerConfig ServerConfig
}

type ServerConfig struct {
	Port                       int
	Protocol                   awsecs.Protocol
	Image                      string
	Debug                      bool
	IngressPort                awsec2.Port
	Version                    string
	Motd                       string
	Difficulty                 string
	MaxPlayers                 string
	AllowNether                string
	AnnouncePlayerAchievements string
	GenerateStructures         string
	Hardcore                   string
	SnooperEnabled             string
	MaxBuildHeight             string
	SpawnAnimals               string
	SpawnMonsters              string
	SpawnNpcs                  string
	Seed                       string
	Mode                       string
	Pvp                        string
	OnlineMode                 string
	ServerName                 string
	EnableWhitelist            string
	Whitelist                  string
	OpPermissionLevel          string
	LevelType                  string
	SpawnProtection            string
	ViewDistance               string
	Icon                       string
	OverrideIcon               string
	OverrideWhitelist          string
}

// NewMinecraftServerStack creates the stack.
func NewMinecraftServerStack(scope constructs.Construct, id string, props *MinecraftServerStackProps) awscdk.Stack {
	var sprops awscdk.StackProps
	if props != nil {
		sprops = props.StackProps
	}

	stack := awscdk.NewStack(scope, &id, &sprops)

	// Create VPC resources with a server-specific security group
	vpcResources := NewVPCResources(stack, fmt.Sprintf("%s-VPC", id), &VPCResourcesProps{
		IngressRule: props.MinecraftServerConfig.IngressPort,
	})

	// Create SNS resources using the provided SnsEmail
	snsresources := NewSNSResources(stack, fmt.Sprintf("%s-SNS", id), &SNSResourcesProps{
		SnsEmail: props.SnsEmail,
	})

	route53Resources := NewRoute53Resources(stack, fmt.Sprintf("%s-Route53", id), &Route53ResourcesProps{
		UsEast1LogGroupArn: props.UsEastLogGroupArn,
		ServerSubDomain:    props.Route53ServerSubDomain,
		Domain:             props.Route53Domain,
		HostedZoneId:       props.Route53HostedZoneId,
	})

	// Add ECS Resources
	ecsResources := NewECSResources(stack, fmt.Sprintf("%s-ECS", id), &ECSResourcesProps{
		CpuSize:               props.EcsCpuSize,
		Domain:                props.Route53Domain,
		EnablePersistence:     props.EcsEnablePersistence,
		HostedZoneId:          props.Route53HostedZoneId,
		MemorySize:            props.EcsMemorySize,
		ServerDebug:           props.MinecraftServerConfig.Debug,
		ServerImage:           props.MinecraftServerConfig.Image,
		ServerPort:            props.MinecraftServerConfig.Port,
		ServerProtocol:        props.MinecraftServerConfig.Protocol,
		ServerSubDomain:       props.Route53ServerSubDomain,
		ShutdownMin:           props.EcsShutdownMin,
		SnsTopic:              snsresources.SnsTopic,
		StartupMin:            props.EcsStartupMin,
		SubDomainHostedZoneId: route53Resources.SubDomainZoneId,
		Vpc:                   vpcResources.Vpc,
		SecurityGroup:         vpcResources.SecurityGroup,

		// Minecraft Server Settings
		MinecraftServerConfig: props.MinecraftServerConfig,
	})

	// Add Lambda Resources
	NewLambdaResources(stack, fmt.Sprintf("%s-Lambda", id), &LambdaResourcesProps{
		QueryLogGroup:   route53Resources.QueryLogGroup,
		Cluster:         ecsResources.Cluster,
		Service:         ecsResources.Service,
		ServerSubDomain: props.Route53ServerSubDomain,
		Domain:          props.Route53Domain,
	})

	return stack
}

// ConfigureServer sets up the server configuration based on edition.
func ConfigureServer(edition, debug string) ServerConfig {
	port := 25565
	protocol := awsecs.Protocol_TCP
	image := "itzg/minecraft-server"

	if edition != "java" {
		port = 19132
		protocol = awsecs.Protocol_UDP
		image = "itzg/minecraft-bedrock-server"
	}

	return ServerConfig{
		Port:                       port,
		Protocol:                   protocol,
		Image:                      image,
		Debug:                      debug == "true",
		IngressPort:                awsec2.Port_Tcp(jsii.Number(float64(port))),
		Version:                    getEnvOrDefault("MINECRAFT_VERSION", "LATEST"),
		Motd:                       getEnvOrDefault("MINECRAFT_MOTD", "Welcome to the on-demand minecraft server!"),
		Difficulty:                 getEnvOrDefault("MINECRAFT_DIFFICULTY", "easy"),
		MaxPlayers:                 getEnvOrDefault("MINECRAFT_MAX_PLAYERS", "20"),
		AllowNether:                getEnvOrDefault("MINECRAFT_ALLOW_NETHER", "true"),
		AnnouncePlayerAchievements: getEnvOrDefault("MINECRAFT_ANNOUNCE_PLAYER_ACHIEVEMENTS", "true"),
		GenerateStructures:         getEnvOrDefault("MINECRAFT_GENERATE_STRUCTURES", "true"),
		Hardcore:                   getEnvOrDefault("MINECRAFT_HARDCORE", "false"),
		SnooperEnabled:             getEnvOrDefault("MINECRAFT_SNOOPER_ENABLED", "true"),
		MaxBuildHeight:             getEnvOrDefault("MINECRAFT_MAX_BUILD_HEIGHT", "256"),
		SpawnAnimals:               getEnvOrDefault("MINECRAFT_SPAWN_ANIMALS", "true"),
		SpawnMonsters:              getEnvOrDefault("MINECRAFT_SPAWN_MONSTERS", "true"),
		SpawnNpcs:                  getEnvOrDefault("MINECRAFT_SPAWN_NPCS", "true"),
		Seed:                       getEnvOrDefault("MINECRAFT_SEED", ""),
		Mode:                       getEnvOrDefault("MINECRAFT_MODE", "survival"),
		Pvp:                        getEnvOrDefault("MINECRAFT_PVP", "true"),
		OnlineMode:                 getEnvOrDefault("MINECRAFT_ONLINE_MODE", "true"),
		ServerName:                 getEnvOrDefault("MINECRAFT_SERVER_NAME", ""),
		EnableWhitelist:            getEnvOrDefault("MINECRAFT_ENABLE_WHITELIST", "false"),
		Whitelist:                  getEnvOrDefault("MINECRAFT_WHITELIST", ""),
		OpPermissionLevel:          getEnvOrDefault("MINECRAFT_OP_PERMISSION_LEVEL", "1"),
		LevelType:                  getEnvOrDefault("MINECRAFT_LEVEL_TYPE", "minecraft:default"),
		SpawnProtection:            getEnvOrDefault("MINECRAFT_SPAWN_PROTECTION", "0"),
		ViewDistance:               getEnvOrDefault("MINECRAFT_VIEW_DISTANCE", "10"),
		Icon:                       getEnvOrDefault("MINECRAFT_ICON", ""),
		OverrideIcon:               getEnvOrDefault("MINECRAFT_OVERRIDE_ICON", "false"),
		OverrideWhitelist:          getEnvOrDefault("MINECRAFT_OVERRIDE_WHITELIST", "false"),
	}
}

// Helper function to get environment variable or default value.
func getEnvOrDefault(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

// ParseEnv retrieves environment variables and configures stack properties.
func ParseEnv() MinecraftServerStackProps {
	return MinecraftServerStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String(getRequiredEnv("AWS_DESTINATION_ACCOUNT")),
				Region:  jsii.String(getRequiredEnv("AWS_DESTINATION_REGION")),
			},
		},
		EcsMinecraftEdition:    getEnvOrDefault("ECS_MINECRAFT_EDITION", "java"),
		Route53ServerSubDomain: getRequiredEnv("ROUTE53_SERVER_SUBDOMAIN"),
		Route53Domain:          getRequiredEnv("ROUTE53_DOMAIN"),
		Route53HostedZoneId:    getRequiredEnv("ROUTE53_HOSTED_ZONE_ID"),
		EcsMemorySize:          getEnvOrDefault("ECS_MEMORY_SIZE", "8192"),
		EcsCpuSize:             getEnvOrDefault("ECS_CPU_SIZE", "4096"),
		SnsEmail:               getRequiredEnv("SNS_EMAIL"),
		EcsStartupMin:          getEnvOrDefault("ECS_STARTUP_MIN", "10"),
		EcsShutdownMin:         getEnvOrDefault("ECS_SHUTDOWN_MIN", "20"),
		EcsDebug:               getEnvOrDefault("ECS_DEBUG", "false"),
		EcsEnablePersistence:   getEnvOrDefault("ECS_ENABLE_PERSISTENCE", "false") == "true",
		MinecraftServerConfig:  ConfigureServer(getEnvOrDefault("ECS_MINECRAFT_EDITION", "java"), getEnvOrDefault("ECS_DEBUG", "false")),
	}
}

// Helper to fetch required environment variables.
func getRequiredEnv(envVar string) string {
	value := os.Getenv(envVar)
	if value == "" {
		log.Fatalf("%s environment variable is required", envVar)
	}
	return value
}

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	stackName := getEnvOrDefault("AWS_STACK_NAME", "MinecraftServerStack")

	// Create Query Log Stack in `us-east-1`
	queryLogStack := NewQueryLogStack(app, fmt.Sprintf("%s-QueryLogGroupStack", stackName), &QueryLogStackProps{
		StackProps:           awscdk.StackProps{Env: &awscdk.Environment{Region: jsii.String("us-east-1")}},
		ServerSubDomain:      os.Getenv("ROUTE53_SERVER_SUBDOMAIN"),
		Domain:               os.Getenv("ROUTE53_DOMAIN"),
		DestinationAccountId: os.Getenv("AWS_DESTINATION_ACCOUNT"),
		DestinationRegion:    os.Getenv("AWS_DESTINATION_REGION"),
	})

	// Create Minecraft Server Stack and set dependency
	minecraftServerStackProps := ParseEnv()
	minecraftServerStackProps.UsEastLogGroupArn = *queryLogStack.QueryLogGroup.LogGroupArn()

	// Create the server stack
	NewMinecraftServerStack(app, stackName, &minecraftServerStackProps)

	app.Synth(nil)
}
