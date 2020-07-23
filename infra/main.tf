data aws_s3_bucket_object fanout {
  bucket = "rmdy-artifacts"
  key    = "rollover/fanout.zip"
}

data aws_s3_bucket_object refresh {
  bucket = "rmdy-artifacts"
  key    = "rollover/refresh.zip"
}
