terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.30"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.13"
    }
  }
}

provider "aws" {
  region = var.region
}

variable "region" {
  type        = string
  description = "AWS region."
  default     = "eu-central-1"
}

variable "cluster_name" {
  type        = string
  description = "Name of the EKS cluster and prefix of the VPC."
  default     = "tamga"
}

variable "cluster_version" {
  type        = string
  description = "Kubernetes version of the EKS cluster, as major.minor (for example the newest version listed as in standard support at https://docs.aws.amazon.com/eks/latest/userguide/kubernetes-versions.html). There is no default on purpose: EKS retires versions, and a default written into this file goes stale."

  validation {
    condition     = can(regex("^1[.][0-9]+$", var.cluster_version))
    error_message = "cluster_version must be a Kubernetes minor version such as 1.33."
  }
}

variable "tamga_admin_key" {
  type        = string
  sensitive   = true
  description = "Management API key of the proxy (TAMGA_ADMIN_KEY). The chart stores it in a Secret."
}

variable "proxy_image_repository" {
  type        = string
  default     = "ghcr.io/yatuk/tamga"
  description = "Image to run. No public image is published: build one from proxy/Dockerfile and push it to a registry the cluster can pull from."
}

variable "proxy_image_tag" {
  type        = string
  description = "Tag of the image you pushed."
}

# The zones the account can use in the region, in name order. Zone suffixes
# differ between regions (not every region has a, b and c), so the names are
# read from AWS and not built from the region name. Local Zones and
# Wavelength Zones need an opt-in and are left out.
data "aws_availability_zones" "available" {
  state = "available"

  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.8"

  name = "${var.cluster_name}-vpc"
  cidr = "10.30.0.0/16"

  # Three subnets of each kind, so three zones. A region with fewer than
  # three such zones fails here, at plan time.
  azs             = slice(data.aws_availability_zones.available.names, 0, 3)
  private_subnets = ["10.30.1.0/24", "10.30.2.0/24", "10.30.3.0/24"]
  public_subnets  = ["10.30.101.0/24", "10.30.102.0/24", "10.30.103.0/24"]

  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true

  tags = {
    "tamga.io/managed-by" = "terraform"
  }
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.8"

  cluster_name    = var.cluster_name
  cluster_version = var.cluster_version

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  cluster_endpoint_public_access = true

  # Without this the identity that runs Terraform has no access to the
  # cluster it just created, and the namespace and the Helm release below
  # are refused.
  enable_cluster_creator_admin_permissions = true

  eks_managed_node_groups = {
    default = {
      min_size       = 2
      max_size       = 10
      desired_size   = 3
      instance_types = ["t3.large"]
      capacity_type  = "ON_DEMAND"
    }
  }

  tags = {
    "tamga.io/managed-by" = "terraform"
  }
}

provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args = [
      "eks",
      "get-token",
      "--cluster-name",
      module.eks.cluster_name,
      "--region",
      var.region,
    ]
  }
}

provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args = [
        "eks",
        "get-token",
        "--cluster-name",
        module.eks.cluster_name,
        "--region",
        var.region,
      ]
    }
  }
}

resource "kubernetes_namespace" "tamga" {
  metadata {
    name = "tamga"
  }
}

resource "helm_release" "tamga_proxy" {
  name       = "tamga-proxy"
  namespace  = kubernetes_namespace.tamga.metadata[0].name
  chart      = "${path.module}/../../helm/tamga-proxy"
  depends_on = [module.eks]

  set_sensitive {
    name  = "secrets.adminKey"
    value = var.tamga_admin_key
  }

  set {
    name  = "image.repository"
    value = var.proxy_image_repository
  }

  set {
    name  = "image.tag"
    value = var.proxy_image_tag
  }
}

output "cluster_endpoint" {
  description = "URL of the cluster's Kubernetes API."
  value       = module.eks.cluster_endpoint
}

output "kubeconfig_command" {
  description = "Command that adds the cluster to your kubeconfig."
  value       = "aws eks update-kubeconfig --name ${module.eks.cluster_name} --region ${var.region}"
}
