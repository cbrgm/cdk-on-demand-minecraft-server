package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	psnet "github.com/shirou/gopsutil/net"
)

const (
	javaPort         = 25565
	rconPort         = 25575
	bedrockPort      = 19132
	bedrockIP        = "127.0.0.1:19132"
	taskMetaEndpoint = "ECS_CONTAINER_METADATA_URI_V4"
	bedrockPingWait  = 1 * time.Second
	checkInterval    = 1 * time.Minute
	rconWaitInterval = 1 * time.Second
	maxStartupWait   = 10 * time.Minute // 600 seconds
)

type Config struct {
	Cluster     string `arg:"env:CLUSTER,required" help:"ECS cluster name"`
	Service     string `arg:"env:SERVICE,required" help:"ECS service name"`
	ServerName  string `arg:"env:SERVERNAME,required" help:"Full A record in Route53"`
	DNSZone     string `arg:"env:DNSZONE,required" help:"Route53 Hosted Zone ID"`
	SNSTopic    string `arg:"env:SNSTOPIC" help:"SNS topic for notifications"`
	StartupMin  int    `arg:"env:STARTUPMIN" default:"10" help:"Startup wait time in minutes"`
	ShutdownMin int    `arg:"env:SHUTDOWNMIN" default:"20" help:"Shutdown wait time in minutes"`
}

func main() {
	var cfg Config
	arg.MustParse(&cfg)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		exitWithError("Failed to load AWS configuration", err, logger)
	}

	ecsClient := ecs.NewFromConfig(awsCfg)
	ec2Client := ec2.NewFromConfig(awsCfg)
	route53Client := route53.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)

	taskID := fetchTaskID(logger)
	publicIP := resolvePublicIP(ecsClient, ec2Client, &cfg, taskID, logger)
	updateDNSRecord(route53Client, &cfg, publicIP, logger)

	edition := determineEdition(&cfg, logger)
	sendStartupNotification(snsClient, &cfg, edition, publicIP, logger)

	if waitForInitialClientConnection(&cfg, edition, logger) {
		monitorClientConnections(ecsClient, snsClient, &cfg, edition, logger)
	} else {
		logger.Info(fmt.Sprintf("%d minutes exceeded without a connection, initiating shutdown.", cfg.StartupMin))
		shutdownService(ecsClient, snsClient, &cfg, logger)
		exitWithError("No initial client connection established, service shut down.", nil, logger)
	}
}

func fetchTaskID(logger *slog.Logger) string {
	taskARN := os.Getenv(taskMetaEndpoint) + "/task"
	resp, err := http.Get(taskARN)
	if err != nil {
		exitWithError("Failed to get task metadata", err, logger)
	}

	// nolint: errcheck
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		exitWithError("Failed to parse task metadata", err, logger)
	}

	if taskID, ok := result["TaskARN"].(string); ok {
		return taskID
	}
	exitWithError("Invalid task ARN received", nil, logger)
	return ""
}

func resolvePublicIP(ecsClient *ecs.Client, ec2Client *ec2.Client, cfg *Config, taskID string, logger *slog.Logger) string {
	resp, err := ecsClient.DescribeTasks(context.TODO(), &ecs.DescribeTasksInput{
		Cluster: aws.String(cfg.Cluster),
		Tasks:   []string{taskID},
	})
	if err != nil || len(resp.Tasks) == 0 {
		exitWithError("Failed to describe ECS task", err, logger)
	}

	var eni string
	for _, detail := range resp.Tasks[0].Attachments[0].Details {
		if detail.Name != nil && *detail.Name == "networkInterfaceId" {
			eni = *detail.Value
			break
		}
	}

	respEC2, err := ec2Client.DescribeNetworkInterfaces(context.TODO(), &ec2.DescribeNetworkInterfacesInput{
		NetworkInterfaceIds: []string{eni},
	})
	if err != nil {
		exitWithError("Failed to describe network interfaces", err, logger)
	}
	return *respEC2.NetworkInterfaces[0].Association.PublicIp
}

func updateDNSRecord(client *route53.Client, cfg *Config, publicIP string, logger *slog.Logger) {
	_, err := client.ChangeResourceRecordSets(context.TODO(), &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(cfg.DNSZone),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String(cfg.ServerName),
						Type: types.RRTypeA,
						TTL:  aws.Int64(30),
						ResourceRecords: []types.ResourceRecord{
							{Value: aws.String(publicIP)},
						},
					},
				},
			},
		},
	})
	if err != nil {
		exitWithError("Failed to update DNS record", err, logger)
	}
	logger.Info("DNS record updated", slog.String("ServerName", cfg.ServerName), slog.String("IP", publicIP))
}

func determineEdition(cfg *Config, logger *slog.Logger) string {
	logger.Info("Determining Minecraft edition based on listening port...")
	counter := 0
	for {
		if isPortOpen(javaPort) {
			logger.Info("Detected Java Edition")
			waitForRCON(logger)
			return "java"
		}
		if isPortOpen(bedrockPort) {
			logger.Info("Detected Bedrock Edition")
			return "bedrock"
		}
		time.Sleep(time.Second)
		counter++
		if counter > int(maxStartupWait.Seconds()) {
			exitWithError("10 minutes elapsed without Minecraft server starting. Terminating.", nil, logger)
		}
	}
}

func isPortOpen(port int) bool {
	conns, _ := psnet.Connections("all")
	for _, conn := range conns {
		if int(conn.Laddr.Port) == port && conn.Status == "LISTEN" {
			return true
		}
	}
	return false
}

func waitForRCON(logger *slog.Logger) {
	logger.Info("Waiting for Minecraft RCON to begin listening...")
	for {
		if isPortOpen(rconPort) {
			logger.Info("RCON is listening, ready for clients.")
			break
		}
		time.Sleep(rconWaitInterval)
	}
}

func waitForInitialClientConnection(cfg *Config, edition string, logger *slog.Logger) bool {
	logger.Info("Checking every 1 minute for active connections to Minecraft...", slog.Int("minutes", cfg.StartupMin))
	for counter := 0; counter < cfg.StartupMin; counter++ {
		if isConnected(edition, logger) {
			logger.Info("Initial connection established, proceeding to shutdown monitoring.")
			return true
		}
		logger.Info(fmt.Sprintf("Waiting for connection, minute %d out of %d...", counter, cfg.StartupMin))
		time.Sleep(checkInterval)
	}
	return false
}

func isConnected(edition string, logger *slog.Logger) bool {
	if edition == "java" {
		return checkConnections(javaPort) > 0
	}
	return sendBedrockPing(logger) > 0
}

func sendBedrockPing(logger *slog.Logger) int {
	conn, err := net.Dial("udp", bedrockIP)
	if err != nil {
		logger.Error("Failed to create UDP connection for Bedrock ping", slog.String("error", err.Error()))
		return 0
	}

	// nolint: errcheck
	defer conn.Close()

	if _, err := conn.Write(buildBedrockPing()); err != nil {
		logger.Error("Failed to send Bedrock ping packet", slog.String("error", err.Error()))
		return 0
	}

	_ = conn.SetReadDeadline(time.Now().Add(bedrockPingWait))
	buffer := make([]byte, 1024)
	n, _ := conn.Read(buffer)

	if len(buffer[:n]) >= 34 {
		parsedResponse := bytes.Split(buffer[34:n], []byte(";"))
		if len(parsedResponse) > 4 {
			return 1
		}
	}
	return 0
}

func buildBedrockPing() []byte {
	ping := append([]byte{0x01}, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x4e, 0x20}...)
	ping = append(ping, []byte{0x00, 0xff, 0xff, 0x00, 0xfe, 0xfe, 0xfe, 0xfe, 0xfd, 0xfd, 0xfd, 0xfd, 0x12, 0x34, 0x56, 0x78}...)
	guid := make([]byte, 8)
	if _, err := rand.Read(guid); err == nil {
		randomHex := make([]byte, hex.EncodedLen(len(guid)))
		hex.Encode(randomHex, guid)
		ping = append(ping, randomHex...)
	}
	return ping
}

func checkConnections(port int) int {
	conns, _ := psnet.Connections("all")
	count := 0
	for _, conn := range conns {
		if int(conn.Laddr.Port) == port && conn.Status == "ESTABLISHED" {
			count++
		}
	}
	return count
}

func monitorClientConnections(ecsClient *ecs.Client, snsClient *sns.Client, cfg *Config, edition string, logger *slog.Logger) {
	logger.Info("Switching to shutdown monitor.")
	counter := 0
	for counter <= cfg.ShutdownMin {
		if !isConnected(edition, logger) {
			logger.Info(fmt.Sprintf("No active connections, %d out of %d minutes", counter, cfg.ShutdownMin))
			counter++
		} else {
			logger.Info("Active connections detected, resetting counter.")
			counter = 0
		}
		time.Sleep(checkInterval)
	}
	logger.Info(fmt.Sprintf("%d minutes elapsed without a connection, terminating.", cfg.ShutdownMin))
	shutdownService(ecsClient, snsClient, cfg, logger)
}

func shutdownService(ecsClient *ecs.Client, snsClient *sns.Client, cfg *Config, logger *slog.Logger) {
	sendShutdownNotification(snsClient, cfg, logger)
	_, err := ecsClient.UpdateService(context.TODO(), &ecs.UpdateServiceInput{
		Cluster:      aws.String(cfg.Cluster),
		Service:      aws.String(cfg.Service),
		DesiredCount: aws.Int32(0),
	})
	if err != nil {
		exitWithError("Failed to set service desired count to zero", err, logger)
	}
	logger.Info("Service shutdown initiated")
}

func sendStartupNotification(client *sns.Client, cfg *Config, edition, publicIP string, logger *slog.Logger) {
	if cfg.SNSTopic == "" {
		return
	}
	message := fmt.Sprintf(
		"Server is online.\nService: %s\nEdition: %s\nAddress: %s (%s)\nCluster: %s\nTime: %s",
		cfg.Service, edition, cfg.ServerName, publicIP, cfg.Cluster, time.Now().Format(time.RFC1123),
	)
	_, _ = client.Publish(context.TODO(), &sns.PublishInput{
		TopicArn: aws.String(cfg.SNSTopic),
		Message:  aws.String(message),
	})
}

func sendShutdownNotification(client *sns.Client, cfg *Config, logger *slog.Logger) {
	if cfg.SNSTopic == "" {
		return
	}
	message := fmt.Sprintf(
		"Shutting down server.\nService: %s\nAddress: %s\nCluster: %s\nTime: %s",
		cfg.Service, cfg.ServerName, cfg.Cluster, time.Now().Format(time.RFC1123),
	)
	_, _ = client.Publish(context.TODO(), &sns.PublishInput{
		TopicArn: aws.String(cfg.SNSTopic),
		Message:  aws.String(message),
	})
}

func exitWithError(msg string, err error, logger *slog.Logger) {
	if err != nil {
		logger.Error(msg, slog.String("error", err.Error()))
	} else {
		logger.Error(msg)
	}
	os.Exit(1)
}
