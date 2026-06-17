###############################################################################
# Smoke test for terraform-provider-nutanix 2.4.3-beta1-fc4
#
# Verifies the application_rule_spec.secured_group_entity_group_reference
# path. Pre-fix the schema required secured_group_category_references on
# the same rule, which conflicted with the v4.2 mutex (MIC-30142):
#   "Failed to create network security policy because the secured group
#    cannot have both category references and entity group reference."
#
# Post-fix the user can use entity_group_reference ALONE.
#
# Requires a reachable Prism Central.
#
# Env vars:
#   TF_VAR_nutanix_user
#   TF_VAR_nutanix_password
#   TF_VAR_nutanix_endpoint
###############################################################################

terraform {
  required_providers {
    nutanix = {
      source  = "nutanix/nutanix"
      version = "2.4.3-beta1-fc4"
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
# Categories the entity_group_v2 selects on (key/value pairs).
###############################################################################
resource "nutanix_category_v2" "tier_web" {
  key         = "fc_eg_tier"
  value       = "web"
  description = "Smoke test category for entity_group fix (fc4)"
}

resource "nutanix_category_v2" "tier_app" {
  key         = "fc_eg_tier"
  value       = "app"
  description = "Smoke test category for entity_group fix (fc4)"
}

###############################################################################
# Entity group: a dynamic set of VMs matching tier:web OR tier:app.
###############################################################################
resource "nutanix_entity_group_v2" "fc_eg_smoke" {
  name        = "fc_eg_smoke"
  type        = "VM"
  description = "Smoke test entity group for fc4 (mutex fix)"

  group_action = "ALLOW"

  entities {
    type      = "CATEGORIES"
    value_uuids = [
      nutanix_category_v2.tier_web.id,
      nutanix_category_v2.tier_app.id,
    ]
  }
}

###############################################################################
# Policy whose application_rule_spec uses secured_group_entity_group_reference
# WITHOUT also supplying secured_group_category_references — the path that
# pre-fix was rejected at plan time (schema validation) AND, even if you
# tried to satisfy the schema by also setting category_references, the
# v4.2 API would have rejected with MIC-30142.
###############################################################################
resource "nutanix_network_security_policy_v2" "fc_eg_policy" {
  name        = "fc_eg_policy_smoke"
  description = "Smoke test policy using entity_group_reference (fc4)"
  type        = "APPLICATION"
  state       = "ENFORCE"
  scope       = "GLOBAL"

  rules {
    type = "APPLICATION"
    spec {
      application_rule_spec {
        # NO secured_group_category_references — pre-fix this was Required.
        secured_group_entity_group_reference = nutanix_entity_group_v2.fc_eg_smoke.id
        src_allow_spec                       = "ALL"
      }
    }
  }
}

output "policy_id" {
  value = nutanix_network_security_policy_v2.fc_eg_policy.id
}

output "entity_group_id" {
  value = nutanix_entity_group_v2.fc_eg_smoke.id
}
