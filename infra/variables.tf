variable "region" {
  description = "AWS region"
  type        = string
  default     = "ap-northeast-1"
}

variable "aws_profile" {
  description = "AWS CLI profile used by the aws/helm providers (null falls back to the default credential chain / AWS_PROFILE env var)"
  type        = string
  default     = null
}

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
  default     = "eks-dd-test"
}

variable "kubernetes_version" {
  description = "Kubernetes version for the EKS control plane"
  type        = string
  default     = "1.31"
}

variable "vpc_name" {
  description = "VPC name"
  type        = string
  default     = "eks-dd-vpc"
}

variable "vpc_cidr" {
  description = "VPC CIDR range"
  type        = string
  default     = "10.0.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "Public subnet CIDR ranges (one per AZ, used by NAT Gateway / ELB)"
  type        = list(string)
  default     = ["10.0.0.0/24", "10.0.1.0/24"]
}

variable "private_subnet_cidrs" {
  description = "Private subnet CIDR ranges (one per AZ, used by EKS worker nodes)"
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.11.0/24"]
}

variable "master_authorized_networks" {
  description = "CIDR blocks allowed to reach the public EKS API endpoint (e.g. [\"203.0.113.4/32\"])"
  type        = list(string)
}

variable "datadog_api_key" {
  description = "Datadog API key (set via terraform.tfvars or TF_VAR_datadog_api_key, never commit the real value)"
  type        = string
  sensitive   = true
}

variable "datadog_site" {
  description = "Datadog site (e.g. datadoghq.com, datadoghq.eu, us3.datadoghq.com, us5.datadoghq.com, ap1.datadoghq.com)"
  type        = string
  default     = "ap1.datadoghq.com"
}
