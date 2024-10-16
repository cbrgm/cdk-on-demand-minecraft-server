package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/alexflint/go-arg"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type Config struct {
	Region  string `arg:"env:REGION,required" help:"AWS region where ECS cluster is located"`
	Cluster string `arg:"env:CLUSTER,required" help:"ECS cluster name"`
	Service string `arg:"env:SERVICE,required" help:"ECS service name"`
}

type LambdaHandler struct {
	Config    Config
	Logger    *slog.Logger
	EcsClient *ecs.Client
}

// NewLambdaHandler initializes a new LambdaHandler.
func NewLambdaHandler() *LambdaHandler {
	// Parse environment variables
	var cfg Config
	arg.MustParse(&cfg)

	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	// Load AWS configuration
	awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(cfg.Region))
	if err != nil {
		logger.Error("Failed to load AWS configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Create ECS client
	ecsClient := ecs.NewFromConfig(awsCfg)

	return &LambdaHandler{
		Config:    cfg,
		Logger:    logger,
		EcsClient: ecsClient,
	}
}

// DescribeService retrieves the ECS service information.
func (h *LambdaHandler) DescribeService(ctx context.Context) (*ecs.DescribeServicesOutput, error) {
	return h.EcsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(h.Config.Cluster),
		Services: []string{h.Config.Service},
	})
}

// UpdateDesiredCount updates the desired count of the ECS service.
func (h *LambdaHandler) UpdateDesiredCount(ctx context.Context, count int32) error {
	_, err := h.EcsClient.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      aws.String(h.Config.Cluster),
		Service:      aws.String(h.Config.Service),
		DesiredCount: aws.Int32(count),
	})
	return err
}

// HandleRequest processes the Lambda event.
func (h *LambdaHandler) HandleRequest(ctx context.Context) error {
	// Describe ECS service
	describeServicesOutput, err := h.DescribeService(ctx)
	if err != nil {
		h.Logger.Error("Failed to describe ECS service", slog.String("error", err.Error()))
		return err
	}

	if len(describeServicesOutput.Services) == 0 {
		h.Logger.Error("No services found", slog.String("cluster", h.Config.Cluster), slog.String("service", h.Config.Service))
		return err
	}

	// Check desired count of the service
	desiredCount := describeServicesOutput.Services[0].DesiredCount
	h.Logger.Info("Current desired count", slog.Int("desiredCount", int(desiredCount)))

	// Update desired count if it's 0
	if desiredCount == 0 {
		err = h.UpdateDesiredCount(ctx, 1)
		if err != nil {
			h.Logger.Error("Failed to update ECS service desired count", slog.String("error", err.Error()))
			return err
		}
		h.Logger.Info("Updated desiredCount to 1")
	} else {
		h.Logger.Info("desiredCount already at 1")
	}

	return nil
}

func main() {
	handler := NewLambdaHandler()
	lambda.Start(handler.HandleRequest)
}
