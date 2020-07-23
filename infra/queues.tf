resource aws_sns_topic rollover {
  name = "ami-builder-notifications"
}

resource aws_sqs_queue rollover {
  name                        = "asg-rollover-events.fifo"
  fifo_queue                  = true
  content_based_deduplication = true
  visibility_timeout_seconds  = 900
}
