package main

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogsdestinations"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type LambdaResourcesProps struct {
	QueryLogGroup   awslogs.LogGroup
	Cluster         awsecs.Cluster
	Service         awsecs.FargateService
	ServerSubDomain string
	Domain          string
}

type LambdaResources struct {
	constructs.Construct
}

func NewLambdaResources(scope constructs.Construct, id string, props *LambdaResourcesProps) *LambdaResources {
	this := constructs.NewConstruct(scope, &id)

	// Create IAM Role for the Lambda function
	lambdaRole := awsiam.NewRole(this, jsii.String(fmt.Sprintf("%s-LambdaRole", id)), &awsiam.RoleProps{
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"ecsPolicy": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Resources: jsii.Strings(
							fmt.Sprintf("%s/*", *props.Service.ServiceArn()),
							*props.Service.ServiceArn(),
							fmt.Sprintf("%s/*", *props.Cluster.ClusterArn()),
							*props.Cluster.ClusterArn(),
						),
						Actions: jsii.Strings("ecs:*"),
					}),
				},
			}),
		},
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")),
		},
	})

	// Create Lambda function using the PROVIDED_AL2023 runtime and x86_64 architecture
	launcherLambda := awslambda.NewFunction(this, jsii.String(fmt.Sprintf("%s-LauncherLambda", id)), &awslambda.FunctionProps{
		FunctionName: jsii.String(fmt.Sprintf("%s-LauncherLambda", id)),
		Code:         awslambda.Code_FromAsset(jsii.String("cmd/lambda/launcher"), nil),
		Role:         lambdaRole,
		Handler:      jsii.String("bootstrap"),
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		LogRetention: awslogs.RetentionDays_ONE_WEEK,
		Environment: &map[string]*string{
			"REGION":  awscdk.Stack_Of(this).Region(),
			"CLUSTER": props.Cluster.ClusterName(),
			"SERVICE": props.Service.ServiceName(),
		},
	})

	// Add permissions for CloudWatch Logs to invoke Lambda
	launcherLambda.AddPermission(jsii.String("InvokeLambda"), &awslambda.Permission{
		Principal: awsiam.NewServicePrincipal(
			jsii.String(fmt.Sprintf("logs.%s.amazonaws.com", *awscdk.Stack_Of(this).Region())), nil),
		Action:        jsii.String("lambda:InvokeFunction"),
		SourceArn:     props.QueryLogGroup.LogGroupArn(),
		SourceAccount: awscdk.Stack_Of(this).Account(),
	})

	// Add CloudWatch Logs subscription filter
	props.QueryLogGroup.AddSubscriptionFilter(jsii.String(fmt.Sprintf("%s-SubscriptionFilter", id)), &awslogs.SubscriptionFilterOptions{
		Destination:   awslogsdestinations.NewLambdaDestination(launcherLambda, &awslogsdestinations.LambdaDestinationOptions{}),
		FilterPattern: awslogs.FilterPattern_AllTerms(jsii.String(fmt.Sprintf("%s.%s", props.ServerSubDomain, props.Domain))),
	})

	return &LambdaResources{
		Construct: this,
	}
}
