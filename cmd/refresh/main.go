package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
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

	return strings.Split(parsed.Service, "/")[1], nil
}

func HandleEvent(ctx context.Context, event events.SQSEvent) error {
	record := event.Records[0]
	var msg RefreshMessage
	if err := json.Unmarshal([]byte(record.Body), &msg); err != nil {
		return fmt.Errorf("unable to parse message JSON: %w", err)
	}

	name, err := msg.ASGName()
	if err != nil {
		return fmt.Errorf("unable to parse ASG name from ARN: %w", err)
	}

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

	_, err = ec2Svc.CreateLaunchTemplateVersion(&ec2.CreateLaunchTemplateVersionInput{
		LaunchTemplateId: asg.LaunchTemplate.LaunchTemplateId,
		SourceVersion:    aws.String("$Latest"),
		LaunchTemplateData: &ec2.RequestLaunchTemplateData{
			ImageId: aws.String(msg.AMI),
		},
	})
	if err != nil {
		return fmt.Errorf("unable to create new launch template version: %w", err)
	}

	refreshReq, err := asgSvc.StartInstanceRefresh(&autoscaling.StartInstanceRefreshInput{
		AutoScalingGroupName: aws.String(name),
	})
	if err != nil {
		return fmt.Errorf("unable to start instance refresh: %w", err)
	}

loop:
	for {
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
			fmt.Printf("%d%% complete", *ref.PercentageComplete)
			fallthrough
		case autoscaling.InstanceRefreshStatusPending:
			time.Sleep(15 * time.Second)
		case autoscaling.InstanceRefreshStatusCancelling, autoscaling.InstanceRefreshStatusCancelled:
			return fmt.Errorf("instance refresh cancelled: %s", *ref.StatusReason)
		case autoscaling.InstanceRefreshStatusSuccessful:
			fmt.Println("Refresh complete")
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
