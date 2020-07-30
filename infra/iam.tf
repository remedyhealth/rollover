### LAMBDAS ###
data aws_iam_policy_document lambda_assume {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

data aws_iam_policy_document refresh {
  statement {
    actions = [
      "autoscaling:DescribeAutoScalingGroups",
      "autoscaling:DescribeInstanceRefreshes",
      "autoscaling:CancelInstanceRefresh",
      "autoscaling:StartInstanceRefresh",
    ]
    resources = ["*"]
  }

  statement {
    actions = [
      "sqs:ReceiveMessage",
      "sqs:DeleteMessage",
      "sqs:GetQueueAttributes",
    ]
    resources = [aws_sqs_queue.rollover.arn]
  }
}

data aws_iam_policy_document fanout {
  statement {
    actions   = ["sqs:SendMessage"]
    resources = [aws_sqs_queue.rollover.arn]
  }
}

resource aws_iam_policy refresh {
  name_prefix = "rollover-refresh"
  policy      = data.aws_iam_policy_document.refresh.json
}

resource aws_iam_policy fanout {
  name_prefix = "rollover-fanout"
  policy      = data.aws_iam_policy_document.fanout.json
}

resource aws_iam_role fanout {
  name_prefix        = "rollover-fanout"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

resource aws_iam_role_policy_attachment fanout_basic_exec {
  role       = aws_iam_role.fanout.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

resource aws_iam_role_policy_attachment fanout {
  role       = aws_iam_role.fanout.name
  policy_arn = aws_iam_policy.fanout.arn
}

resource aws_iam_role refresh {
  name_prefix        = "rollover-refresh"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

resource aws_iam_role_policy_attachment refresh_basic_exec {
  role       = aws_iam_role.refresh.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole"
}

resource aws_iam_role_policy_attachment refresh {
  role       = aws_iam_role.refresh.name
  policy_arn = aws_iam_policy.refresh.arn
}

### SNS ###
data aws_iam_policy_document sns_assume {
  statment {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["sns.amazonaws.com"]
    }
  }
}

resource aws_iam_role sns {
  name_prefix        = "rollover-sns"
  assume_role_policy = data.aws_iam_policy_document.sns_assume.json
}

resource aws_iam_role_policy_attachment sns {
  role       = aws_iam_role.sns.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonSNSRole"
}
