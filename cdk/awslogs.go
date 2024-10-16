package main

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogsdestinations"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type QueryLogStackProps struct {
	awscdk.StackProps
	ServerSubDomain      string
	Domain               string
	DestinationAccountId string // Account ID to construct the destination ARN
	DestinationRegion    string // Destination region (e.g., "eu-central-1")
}

type QueryLogStack struct {
	awscdk.Stack
	QueryLogGroup awslogs.LogGroup
}

func NewQueryLogStack(scope constructs.Construct, id string, props *QueryLogStackProps) *QueryLogStack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	// Create the log group for Route53 query logs in `us-east-1`
	logGroupName := fmt.Sprintf("/aws/route53/%s.%s", props.ServerSubDomain, props.Domain)
	queryLogGroupID := fmt.Sprintf("%s-QueryLogGroup", id)
	queryLogGroup := awslogs.NewLogGroup(stack, jsii.String(queryLogGroupID), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(logGroupName),
		Retention:     awslogs.RetentionDays_THREE_DAYS,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	// Create resource policy for allowing Route53 to log to CloudWatch
	resourcePolicyID := fmt.Sprintf("%s-ResourcePolicy", id)
	awslogs.NewResourcePolicy(stack, jsii.String(resourcePolicyID), &awslogs.ResourcePolicyProps{
		ResourcePolicyName: jsii.String(resourcePolicyID),
		PolicyStatements: &[]awsiam.PolicyStatement{
			awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
				Principals: &[]awsiam.IPrincipal{
					awsiam.NewServicePrincipal(jsii.String("route53.amazonaws.com"), nil),
				},
				Actions: &[]*string{
					jsii.String("logs:CreateLogStream"),
					jsii.String("logs:PutLogEvents"),
					jsii.String("logs:DescribeLogStreams"),
					jsii.String("logs:CreateLogGroup"),
				},
				Resources: &[]*string{
					jsii.String(fmt.Sprintf("%s:*", *queryLogGroup.LogGroupArn())),
				},
			}),
		},
	})
	// IAM Role for Lambda to forward logs
	lambdaRoleID := fmt.Sprintf("%s-LogForwarderRole", id)
	lambdaRole := awsiam.NewRole(stack, jsii.String(lambdaRoleID), &awsiam.RoleProps{
		RoleName:  jsii.String(lambdaRoleID),
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("lambda.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(jsii.String("service-role/AWSLambdaBasicExecutionRole")),
		},
		InlinePolicies: &map[string]awsiam.PolicyDocument{
			"AllowLogForwarding": awsiam.NewPolicyDocument(&awsiam.PolicyDocumentProps{
				Statements: &[]awsiam.PolicyStatement{
					awsiam.NewPolicyStatement(&awsiam.PolicyStatementProps{
						Actions: &[]*string{
							jsii.String("logs:PutLogEvents"),
							jsii.String("logs:CreateLogStream"),
						},
						Resources: &[]*string{
							jsii.String(fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s:*", props.DestinationRegion, props.DestinationAccountId, logGroupName)),
						},
					}),
				},
			}),
		},
	})

	// Create Lambda function to forward logs
	logForwarderLambdaID := fmt.Sprintf("%s-LogForwarderLambda", id)
	logForwarderLambda := awslambda.NewFunction(stack, jsii.String(logForwarderLambdaID), &awslambda.FunctionProps{
		FunctionName: jsii.String(logForwarderLambdaID),
		Code:         awslambda.Code_FromAsset(jsii.String("cmd/lambda/logforwarder"), nil),
		Role:         lambdaRole,
		Handler:      jsii.String("bootstrap"),
		Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
		Architecture: awslambda.Architecture_ARM_64(),
		LogRetention: awslogs.RetentionDays_ONE_WEEK,
		Environment: &map[string]*string{
			"TARGET_LOG_GROUP_ARN":  jsii.String(fmt.Sprintf("arn:aws:logs:%s:%s:log-group:%s:*", props.DestinationRegion, props.DestinationAccountId, logGroupName)),
			"TARGET_LOG_GROUP_NAME": queryLogGroup.LogGroupName(),
		},
	})

	// Add CloudWatch Logs subscription filter
	subscriptionFilterID := fmt.Sprintf("%s-SubscriptionFilter", id)
	queryLogGroup.AddSubscriptionFilter(jsii.String(subscriptionFilterID), &awslogs.SubscriptionFilterOptions{
		Destination: awslogsdestinations.NewLambdaDestination(logForwarderLambda, &awslogsdestinations.LambdaDestinationOptions{}),
		FilterPattern: awslogs.FilterPattern_AllTerms(
			jsii.String("_minecraft._tcp"),
			jsii.String("SRV"),
			jsii.String(fmt.Sprintf("%s.%s", props.ServerSubDomain, props.Domain)),
		),
	})
	return &QueryLogStack{
		Stack:         stack,
		QueryLogGroup: queryLogGroup,
	}
}
