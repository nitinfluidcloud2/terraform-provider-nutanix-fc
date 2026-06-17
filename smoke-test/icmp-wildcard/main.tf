###############################################################################
# Smoke test for terraform-provider-nutanix 2.4.3-beta1-fc3
#
# Verifies the ICMP wildcard fix is live: HCL that specifies ONLY
# `icmp_services { is_all_allowed = true }` (without type / code) must apply
# successfully. Pre-fix the provider serialized type=0 / code=0 into the
# payload and the backend rejected with:
#   MIC-30302 on nutanix_service_groups_v2
#   MIC-30113 on nutanix_network_security_policy_v2 (inline rule)
#
# Requires a reachable Prism Central.
#
# Env vars:
#   TF_VAR_nutanix_user
#   TF_VAR_nutanix_password
#   TF_VAR_nutanix_endpoint
#
# Run:
#   export TF_CLI_CONFIG_FILE=$(realpath ../terraformrc)
#   terraform init -reconfigure
#   terraform apply -auto-approve   # must succeed for both resources
#   terraform destroy -auto-approve
###############################################################################

terraform {
  required_providers {
    nutanix = {
      source  = "nutanix/nutanix"
      version = "2.4.3-beta1-fc3"
    }
  }
}

provider "nutanix" {
  username = var.nutanix_user
  password = var.nutanix_password
  endpoint = var.nutanix_endpoint
  insecure = true
  port     = 9440
}

variable "nutanix_user" {
  type = string
}

variable "nutanix_password" {
  type      = string
  sensitive = true
}

variable "nutanix_endpoint" {
  type = string
}

###############################################################################
# Scenario A — standalone service group with ICMP wildcard
# Pre-fix: MIC-30302
###############################################################################
resource "nutanix_service_groups_v2" "icmp_all_smoke" {
  name        = "fc_icmp_all_smoke"
  description = "Smoke test for ICMP wildcard fix (fc3)"
  icmp_services {
    is_all_allowed = true
  }
}

###############################################################################
# Scenario B — inline icmp_services on a network security policy rule
# Pre-fix: MIC-30113
###############################################################################
resource "nutanix_category_v2" "icmp_smoke_grp" {
  key         = "fc_icmp_smoke_tier"
  value       = "app"
  description = "Category for ICMP wildcard smoke test (fc3)"
}

resource "nutanix_network_security_policy_v2" "icmp_wildcard_smoke" {
  name        = "fc_icmp_wildcard_smoke"
  description = "Smoke test policy for ICMP wildcard fix (fc3)"
  type        = "APPLICATION"
  state       = "ENFORCE"
  scope       = "GLOBAL"

  rules {
    type = "APPLICATION"
    spec {
      application_rule_spec {
        secured_group_category_references = [nutanix_category_v2.icmp_smoke_grp.id]
        src_allow_spec                    = "ALL"
        icmp_services {
          is_all_allowed = true
        }
      }
    }
  }
}

output "service_group_id" {
  value = nutanix_service_groups_v2.icmp_all_smoke.id
}

output "policy_id" {
  value = nutanix_network_security_policy_v2.icmp_wildcard_smoke.id
}
