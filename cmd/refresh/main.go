package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/remedyhealth/rollover/shared"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type RefreshMessage struct {
	ARN string `json:"arn"`
	AMI string `json:"ami"`
}

func (msg RefreshMessage) ASGName() (string, error) {
	parsed, err := arn.Parse(msg.ARN)
	if err != nil {
		return "", err
	}

	log.Debug().Dict("arn", zerolog.Dict().
		Str("account_id", parsed.AccountID).
		Str("partition", parsed.Partition).
		Str("region", parsed.Region).
		Str("resource", parsed.Resource).
		Str("service", parsed.Service),
	).Msg("Parsed")

	return strings.Split(parsed.Resource, "/")[1], nil
}

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if os.Getenv("DEBUG") == "1" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	log.Info().
		Str("version", shared.Version).
		Str("build", shared.BuildNum).
		Str("commit", shared.Rev).
		Msg("Init")
}

func HandleEvent(ctx context.Context, event events.SQSEvent) error {
	record := event.Records[0]
	log.Debug().RawJSON("event", []byte(record.Body)).Msg("Trigger")

	var msg RefreshMessage
	if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
		return fmt.Errorf("unable to parse message JSON: %w", err)
	}
	log.Info().Str("arn", msg.ARN).Str("ami", msg.AMI).Msg("Parsed event")

	name, err := msg.ASGName()
	if err != nil {
		return fmt.Errorf("unable to parse ASG name from ARN: %w", err)
	}
	log.Debug().Str("asg_name", name).Msg("Parsed")

	sess, err := session.NewSession()
	if err != nil {
		return fmt.Errorf("unable to create AWS session: %w", err)
	}

	ec2Svc := ec2.New(sess)
	asgSvc := autoscaling.New(sess)

	asgDesc, err := asgSvc.DescribeAutoScalingGroups(&autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: aws.StringSlice([]string{name}),
		MaxRecords:            aws.Int64(1),
	})
	if err != nil {
		return fmt.Errorf("unable to describe ASG: %w", err)
	}
	asg := asgDesc.AutoScalingGroups[0]

	out, err := ec2Svc.CreateLaunchTemplateVersion(&ec2.CreateLaunchTemplateVersionInput{
		LaunchTemplateId: asg.LaunchTemplate.LaunchTemplateId,
		SourceVersion:    aws.String("$Latest"),
		LaunchTemplateData: &ec2.RequestLaunchTemplateData{
			ImageId: aws.String(msg.AMI),
		},
	})
	if err != nil {
		return fmt.Errorf("unable to create new launch template version: %w", err)
	}
	log.Info().
		Int64("num", *out.LaunchTemplateVersion.VersionNumber).
		Msg("Created LaunchTemplate version")

	refreshReq, err := asgSvc.StartInstanceRefresh(&autoscaling.StartInstanceRefreshInput{
		AutoScalingGroupName: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("unable to start instance refresh: %w", err)
	}
	log.Info().Str("id", *refreshReq.InstanceRefreshId).Msg("Started refresh")

	deadline, _ := ctx.Deadline()
	deadline = deadline.Add(-100 * time.Millisecond)
	timeoutChan := time.After(time.Until(deadline))

loop:
	for {
		select {
		case <-timeoutChan:
			log.Warn().Msg("Lambda about to time out, but refresh is still running")
			return nil
		default:
		}

		refresh, err := asgSvc.DescribeInstanceRefreshes(&autoscaling.DescribeInstanceRefreshesInput{
			AutoScalingGroupName: aws.String(name),
			InstanceRefreshIds:   aws.StringSlice([]string{*refreshReq.InstanceRefreshId}),
			MaxRecords:           aws.Int64(1),
		})
		if err != nil {
			return fmt.Errorf("unable to query for instance refresh status: %w", err)
		}
		ref := refresh.InstanceRefreshes[0]

		switch *ref.Status {
		case autoscaling.InstanceRefreshStatusFailed:
			return fmt.Errorf("refresh failed: %s", *ref.StatusReason)
		case autoscaling.InstanceRefreshStatusInProgress:
			log.Info().Int64("complete", *ref.PercentageComplete).Msg("Progress")
			time.Sleep(time.Minute)
		case autoscaling.InstanceRefreshStatusPending:
			log.Debug().Msg("Refresh pending, sleeping for 15 seconds")
			time.Sleep(15 * time.Second)
		case autoscaling.InstanceRefreshStatusCancelling, autoscaling.InstanceRefreshStatusCancelled:
			return fmt.Errorf("instance refresh cancelled: %s", *ref.StatusReason)
		case autoscaling.InstanceRefreshStatusSuccessful:
			log.Info().Msg("Complete")
			break loop
		default:
			return fmt.Errorf("unknown instance refresh status: %s", *ref.Status)
		}
	}

	return nil
}

func main() {
	lambda.Start(HandleEvent)
}
