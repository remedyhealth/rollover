resource aws_lambda_function refresh {
  function_name                  = "rollover-refresh"
  description                    = "Trigger ASG instance refresh from the queue"
  role                           = aws_iam_role.refresh.arn
  handler                        = "refresh"
  runtime                        = "go1.x"
  timeout                        = 900
  reserved_concurrent_executions = 1
  s3_bucket                      = "rmdy-artifacts"
  s3_key                         = "rollover/refresh.zip"
  s3_object_version              = data.aws_s3_bucket_object.refresh.version_id
}

resource aws_lambda_event_source_mapping refresh {
  event_source_arn = aws_sqs_queue.rollover.arn
  function_name    = aws_lambda_function.refresh.arn
  batch_size       = 1
}
