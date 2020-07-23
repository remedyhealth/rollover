terraform {
  required_version = "~> 0.12"

  backend remote {
    hostname     = "app.terraform.io"
    organization = "Remedy-Health-Media"

    workspaces {
      prefix = "rollover-"
    }
  }
}

provider aws {
  version = "~> 2.60"
  region  = "us-east-1"
}
