package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sts"
	consul "github.com/hashicorp/consul/api"
)

const (
	manifestPath = "packer_manifest"
	consulPrefix = "rollover/groups"

	// just a random uuidgen
	groupID = "8FA07FBE-2772-4BFA-ABE7-70E0F2398ABF"

	allTypes = "all"
)

var queueURL = os.Getenv("QUEUE_URL")

type BuildNotification struct {
	Type string `json:"type"`
}

type PackerBuild struct {
	Name string `json:"name"`
	AMI  string `json:"artifact_id"`
}

type Manifest struct {
	Builds []*PackerBuild `json:"builds"`
}

type ASGConfig struct {
	ARN   string `json:"arn"`
	Type  string `json:"ami_type"`
	Order uint   `json:"order"`
}

type ByOrder []ASGConfig

func (o ByOrder) Len() int           { return len(o) }
func (o ByOrder) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }
func (o ByOrder) Less(i, j int) bool { return o[i].Order < o[j].Order }

type RolloverMesssage struct {
	ARN string `json:"arn"`
	AMI string `json:"ami"`
}

func HandleEvent(ctx context.Context, event events.SNSEvent) error {
	var notif BuildNotification

	if err := json.Unmarshal([]byte(event.Records[0].SNS.Message), &notif); err != nil {
		return fmt.Errorf("unable to parse notification JSON: %w", err)
	}

	session, err := session.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create AWS session: %w", err)
	}

	stsClient := sts.New(session)
	ident, err := stsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("unable to get AWS identity: %w", err)
	}
	accountID := *ident.Account

	consulClient, err := consul.NewClient(consul.DefaultConfig())
	if err != nil {
		return fmt.Errorf("unable to connect to Consul: %w", err)
	}

	kv := consulClient.KV()
	manifestKey, _, err := kv.Get(manifestPath, nil)
	if err != nil {
		return fmt.Errorf("unable to get manifest from Consul: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestKey.Value, &manifest); err != nil {
		return fmt.Errorf("unable to parse manifest JSON: %w", err)
	}

	amis := make(map[string]string, len(manifest.Builds))
	for _, build := range manifest.Builds {
		amis[build.Name] = strings.TrimPrefix(build.AMI, "us-east-1:")
	}

	asgKVs, _, err := kv.List(fmt.Sprintf("%s/%s/", consulPrefix, accountID), nil)
	if err != nil {
		return fmt.Errorf("unable to list ASG configs: %w", err)
	}
	asgConfigs := make([]ASGConfig, 0)
	for _, kvPair := range asgKVs {
		if strings.HasSuffix(kvPair.Key, "/") {
			continue
		}

		var config ASGConfig
		if err := json.Unmarshal(kvPair.Value, &config); err != nil {
			return fmt.Errorf("unable to parse config JSON at %s: %w", kvPair.Key, err)
		}

		if notif.Type != config.Type && notif.Type != allTypes {
			continue
		}

		// If the order is not set, set it to the max to ensure that a zero value
		// won't break the intended ordering.
		if config.Order == 0 {
			config.Order = math.MaxUint32
		}

		asgConfigs = append(asgConfigs, config)
	}
	sort.Sort(ByOrder(asgConfigs))

	sqsClient := sqs.New(session)

	for _, config := range asgConfigs {
		ami, ok := amis[config.Type]
		if !ok {
			return fmt.Errorf("unable to find AMI of type %s for ASG %s", config.Type, config.ARN)
		}

		msg := RolloverMesssage{
			AMI: ami,
			ARN: config.ARN,
		}

		encoded, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("unable to encode SQS message: %w", err)
		}

		out, err := sqsClient.SendMessage(&sqs.SendMessageInput{
			MessageGroupId: aws.String(groupID),
			MessageBody:    aws.String(string(encoded)),
			QueueUrl:       aws.String(queueURL),
		})

		if err != nil {
			return fmt.Errorf("unable to send SQS message: %w", err)
		}

		fmt.Printf("Queued %s: %s => %s", *out.MessageId, config.ARN, ami)
	}

	return nil
}

func main() {
	lambda.Start(HandleEvent)
}
