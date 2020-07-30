resource aws_sns_topic rollover {
  name                                = "ami-builder-notifications"
  lambda_success_feedback_sample_rate = 100
  lambda_failure_feedback_role_arn    = aws_iam_role.sns.arn
  lambda_success_feedback_role_arn    = aws_iam_role.sns.arn
}

resource aws_sqs_queue rollover {
  name                        = "asg-rollover-events.fifo"
  fifo_queue                  = true
  content_based_deduplication = true
  visibility_timeout_seconds  = 900
}
