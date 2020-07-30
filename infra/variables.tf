variable consul_token {
  description = "Consul ACL token"
  type        = string
}

variable lambda_debug {
  description = "Whether to enable lambda debug logging"
  type        = bool
  default     = false
}
