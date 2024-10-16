package main

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2/awssns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awssnssubscriptions"
	"github.com/aws/constructs-go/constructs/v10"
	"github.com/aws/jsii-runtime-go"
)

type SNSResourcesProps struct {
	SnsEmail string
}

type SNSResources struct {
	constructs.Construct
	SnsTopic awssns.Topic
}

func NewSNSResources(scope constructs.Construct, id string, props *SNSResourcesProps) *SNSResources {
	this := constructs.NewConstruct(scope, &id)

	snsTopicID := fmt.Sprintf("%s-SnsTopic", id)
	snsTopic := awssns.NewTopic(this, jsii.String(snsTopicID), &awssns.TopicProps{
		TopicName: jsii.String(snsTopicID),
	})

	emailSubscription := awssnssubscriptions.NewEmailSubscription(jsii.String(props.SnsEmail), &awssnssubscriptions.EmailSubscriptionProps{})
	snsTopic.AddSubscription(emailSubscription)

	return &SNSResources{
		Construct: this,
		SnsTopic:  snsTopic,
	}
}
