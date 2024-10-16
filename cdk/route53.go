package main

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsroute53"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type Route53ResourcesProps struct {
	ServerSubDomain    string
	Domain             string
	HostedZoneId       string
	UsEast1LogGroupArn string
}

type Route53Resources struct {
	constructs.Construct
	QueryLogGroup   awslogs.LogGroup
	SubDomainZoneId string
}

func NewRoute53Resources(scope constructs.Construct, id string, props *Route53ResourcesProps) *Route53Resources {
	this := constructs.NewConstruct(scope, &id)

	// Create the log group for Route53 query logs
	logGroupName := fmt.Sprintf("/aws/route53/%s.%s", props.ServerSubDomain, props.Domain)
	queryLogGroup := awslogs.NewLogGroup(this, jsii.String(fmt.Sprintf("%s-QueryLogGroup", id)), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(logGroupName),
		Retention:     awslogs.RetentionDays_THREE_DAYS,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	// Update the resource policy to allow Route 53 to write to the log group in us-east-1
	resourcePolicyName := fmt.Sprintf("%s-Route53ResourcePolicy", id)
	awslogs.NewResourcePolicy(this, jsii.String(resourcePolicyName), &awslogs.ResourcePolicyProps{
		ResourcePolicyName: jsii.String(resourcePolicyName),
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
					jsii.String(fmt.Sprintf("%s:*", props.UsEast1LogGroupArn)),
				},
			}),
		},
	})

	// Reference the hosted zone
	hostedZone := awsroute53.HostedZone_FromHostedZoneAttributes(this, jsii.String(fmt.Sprintf("%s-HostedZone", id)), &awsroute53.HostedZoneAttributes{
		ZoneName:     jsii.String(props.Domain),
		HostedZoneId: jsii.String(props.HostedZoneId),
	})

	// Create a hosted zone for the subdomain
	subdomainHostedZoneName := fmt.Sprintf("%s-SubdomainHostedZone", id)
	subdomainHostedZone := awsroute53.NewHostedZone(this, jsii.String(subdomainHostedZoneName), &awsroute53.HostedZoneProps{
		ZoneName:             jsii.String(fmt.Sprintf("%s.%s", props.ServerSubDomain, props.Domain)),
		QueryLogsLogGroupArn: &props.UsEast1LogGroupArn,
	})

	// Create the NS record for the subdomain
	nsRecordName := fmt.Sprintf("%s-SubdomainNsRecord", id)
	awsroute53.NewNsRecord(this, jsii.String(nsRecordName), &awsroute53.NsRecordProps{
		Zone:       hostedZone,
		Values:     subdomainHostedZone.HostedZoneNameServers(),
		RecordName: jsii.String(fmt.Sprintf("%s.%s", props.ServerSubDomain, props.Domain)),
	})

	// Create an A record for the subdomain
	aRecordName := fmt.Sprintf("%s-ARecord", id)
	awsroute53.NewARecord(this, jsii.String(aRecordName), &awsroute53.ARecordProps{
		Zone:       subdomainHostedZone,
		Target:     awsroute53.RecordTarget_FromIpAddresses(jsii.String("192.168.1.1")),
		Ttl:        awscdk.Duration_Seconds(jsii.Number(30)),
		RecordName: jsii.String(fmt.Sprintf("%s.%s", props.ServerSubDomain, props.Domain)),
	})

	return &Route53Resources{
		Construct:       this,
		QueryLogGroup:   queryLogGroup,
		SubDomainZoneId: *subdomainHostedZone.HostedZoneId(),
	}
}
