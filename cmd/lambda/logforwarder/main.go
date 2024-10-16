package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

type LogForwarder struct {
	LogGroupName string
	LogsClient   *cloudwatchlogs.Client
	Logger       *slog.Logger
}

// CloudWatch Log Event Structure.
type CloudWatchLogEvent struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

// Log Event Data Structure.
type LogData struct {
	MessageType string               `json:"messageType"`
	LogEvents   []CloudWatchLogEvent `json:"logEvents"`
	LogGroup    string               `json:"logGroup"`
	LogStream   string               `json:"logStream"`
}

func NewLogForwarder() *LogForwarder {
	// Get the target log group name or ARN from environment variables
	logGroupName := os.Getenv("TARGET_LOG_GROUP_NAME")
	logGroupArn := os.Getenv("TARGET_LOG_GROUP_ARN")

	if logGroupName == "" && logGroupArn == "" {
		slog.Error("Missing environment variable: either TARGET_LOG_GROUP_NAME or TARGET_LOG_GROUP_ARN must be set")
		os.Exit(1)
	}

	// If logGroupName is not directly provided, extract it from the ARN
	if logGroupName == "" {
		logGroupName = extractLogGroupName(logGroupArn)
	}

	// Load AWS SDK configuration
	awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("eu-central-1"))
	if err != nil {
		slog.Error("Failed to load AWS configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Create CloudWatch Logs client
	logsClient := cloudwatchlogs.NewFromConfig(awsCfg)

	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	return &LogForwarder{
		LogGroupName: logGroupName,
		LogsClient:   logsClient,
		Logger:       logger,
	}
}

// Extract log group name from ARN.
func extractLogGroupName(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return ""
	}
	// Extract the log group name part from the ARN
	return strings.TrimPrefix(parts[5], "log-group:")
}

// decompressGzip decompresses gzipped data.
func decompressGzip(data []byte) ([]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	// nolint: errcheck
	defer gzipReader.Close()

	var decompressed bytes.Buffer
	_, err = io.Copy(&decompressed, gzipReader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return decompressed.Bytes(), nil
}

// generateUniqueLogStreamName creates a unique name using timestamp and a random suffix.
func generateUniqueLogStreamName() string {
	timestamp := time.Now().UnixNano()
	randomSuffix := rand.Intn(100000)
	return fmt.Sprintf("forwarded-%d-%d", timestamp, randomSuffix)
}

// HandleRequest processes the incoming CloudWatch Logs and forwards them.
func (f *LogForwarder) HandleRequest(ctx context.Context, event events.CloudwatchLogsEvent) error {
	// Decode the CloudWatch Logs data
	decodedData, err := base64.StdEncoding.DecodeString(event.AWSLogs.Data)
	if err != nil {
		f.Logger.Error("Failed to decode log data", slog.String("error", err.Error()))
		return err
	}

	// Decompress the gzipped log data
	decompressedData, err := decompressGzip(decodedData)
	if err != nil {
		f.Logger.Error("Failed to decompress log data", slog.String("error", err.Error()))
		return err
	}

	// Unmarshal the log event data
	var logData LogData
	err = json.Unmarshal(decompressedData, &logData)
	if err != nil {
		f.Logger.Error("Failed to unmarshal log data", slog.String("error", err.Error()))
		return err
	}

	// Prepare log events to send to the target log group
	for _, logEvent := range logData.LogEvents {
		err = f.forwardLogEvent(ctx, logEvent)
		if err != nil {
			f.Logger.Error("Failed to forward log event", slog.String("eventID", logEvent.ID), slog.String("logGroupName", f.LogGroupName), slog.String("error", err.Error()))
		}
	}

	return nil
}

// forwardLogEvent sends a single log event to the target log group.
func (f *LogForwarder) forwardLogEvent(ctx context.Context, event CloudWatchLogEvent) error {
	// Create a unique log stream name to avoid conflicts
	logStreamName := generateUniqueLogStreamName()

	// Attempt to create the log stream in the target log group
	_, err := f.LogsClient.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  aws.String(f.LogGroupName),
		LogStreamName: aws.String(logStreamName),
	})
	if err != nil {
		if !strings.Contains(err.Error(), "ResourceAlreadyExistsException") {
			f.Logger.Error("Failed to create log stream", slog.String("logStreamName", logStreamName), slog.String("logGroupName", f.LogGroupName), slog.String("error", err.Error()))
			return err
		}
		// If the log stream already exists, proceed
		f.Logger.Info("Log stream already exists, proceeding to put log events", slog.String("logStreamName", logStreamName))
	}

	// Send the log event to the created log stream
	_, err = f.LogsClient.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogEvents: []types.InputLogEvent{
			{
				Message:   aws.String(event.Message),
				Timestamp: aws.Int64(event.Timestamp),
			},
		},
		LogGroupName:  aws.String(f.LogGroupName),
		LogStreamName: aws.String(logStreamName),
	})
	if err != nil {
		f.Logger.Error("Failed to put log event", slog.String("logStreamName", logStreamName), slog.String("logGroupName", f.LogGroupName), slog.String("eventID", event.ID), slog.String("error", err.Error()))
		return err
	}

	f.Logger.Info("Successfully forwarded log event", slog.String("logStreamName", logStreamName), slog.String("logGroupName", f.LogGroupName), slog.String("eventID", event.ID))
	return nil
}

func main() {
	forwarder := NewLogForwarder()
	lambda.Start(forwarder.HandleRequest)
}
