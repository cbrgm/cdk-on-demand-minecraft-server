package main

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type VPCResourcesProps struct {
	IngressRule awsec2.Port
}

type VPCResources struct {
	constructs.Construct
	SecurityGroup awsec2.SecurityGroup
	Vpc           awsec2.Vpc
}

func NewVPCResources(scope constructs.Construct, id string, props *VPCResourcesProps) *VPCResources {
	this := constructs.NewConstruct(scope, &id)

	// Create VPC
	vpcID := fmt.Sprintf("%s-VPC", id)
	vpc := awsec2.NewVpc(this, jsii.String(vpcID), &awsec2.VpcProps{
		VpcName:     jsii.String(vpcID),
		NatGateways: jsii.Number(0),
		SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
			{
				CidrMask:            jsii.Number(24),
				Name:                jsii.String(fmt.Sprintf("%s-PublicSubnet", id)),
				SubnetType:          awsec2.SubnetType_PUBLIC,
				MapPublicIpOnLaunch: jsii.Bool(true),
			},
		},
		MaxAzs: jsii.Number(2),
	})

	// Create Security Group
	sgID := fmt.Sprintf("%s-SecurityGroup", id)
	sg := awsec2.NewSecurityGroup(this, jsii.String(sgID), &awsec2.SecurityGroupProps{
		Vpc:               vpc,
		SecurityGroupName: jsii.String(sgID),
		Description:       jsii.String("Security Group for server"),
		AllowAllOutbound:  jsii.Bool(true),
	})

	// Add ingress rule
	sg.AddIngressRule(
		awsec2.Peer_AnyIpv4(),
		props.IngressRule,
		jsii.String(fmt.Sprintf("%s-AllowServerTraffic", id)),
		jsii.Bool(false),
	)

	return &VPCResources{
		Construct:     this,
		SecurityGroup: sg,
		Vpc:           vpc,
	}
}
