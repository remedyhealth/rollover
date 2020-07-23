resource aws_lambda_function fanout {
  function_name     = "rollover-fanout"
  description       = "Fan out AMI build notifications into the queue to refresh ASGs"
  role              = aws_iam_role.fanout.arn
  handler           = "fanout"
  runtime           = "go1.x"
  timeout           = 30
  s3_bucket         = "rmdy-artifacts"
  s3_key            = "rollover/fanout.zip"
  s3_object_version = data.aws_s3_bucket_object.fanout.version_id

  environment {
    variables = {
      CONSUL_HTTP_ADDR  = "https://consul.rmdy.hm"
      CONSUL_HTTP_TOKEN = var.consul_token
      QUEUE_URL         = aws_sqs_queue.rollover.id
    }
  }
}

resource aws_lambda_permission allow_sns {
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.fanout.function_name
  principal     = "sns.amazonaws.com"
  source_arn    = aws_sns_topic.rollover.arn
}

resource aws_sns_topic_subscription fanout {
  topic_arn              = aws_sns_topic.rollover.arn
  protocol               = "lambda"
  endpoint               = aws_lambda_function.fanout.arn
  endpoint_auto_confirms = true
}
