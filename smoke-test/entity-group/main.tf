###############################################################################
# Smoke test for terraform-provider-nutanix 2.4.3-beta1-fc4
#
# Verifies the application_rule_spec.secured_group_entity_group_reference
# path now works without supplying secured_group_category_references on the
# same rule. Pre-fix the schema required the category list, which conflicted
# with the v4.2 mutex (MIC-30142). Post-fix the user can use
# entity_group_reference alone.
#
# Resource shapes match ~/go/fcdeploy/kodata/nutanix_schema.json (the
# authoritative provider schema).
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
# Two categories — the entity_group will select VMs tagged with either of these.
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
# Entity group — VMs that match either tier_web OR tier_app categories.
#
# Schema shape per ~/go/fcdeploy/kodata/nutanix_schema.json:
#   nutanix_entity_group_v2 {
#     name                                    # required
#     description                             # optional
#     allowed_config { entities { type, selected_by, reference_ext_ids } }
#   }
# (no top-level `type`, no `group_action`, no top-level `entities` block).
###############################################################################
resource "nutanix_entity_group_v2" "fc_eg_smoke" {
  name        = "fc_eg_smoke"
  description = "Smoke test entity group for fc4 (mutex fix)"

  allowed_config {
    entities {
      type        = "VM"
      selected_by = "CATEGORY_EXT_ID"
      reference_ext_ids = [
        nutanix_category_v2.tier_web.id,
        nutanix_category_v2.tier_app.id,
      ]
    }
  }
}

###############################################################################
# Policy whose application_rule_spec uses secured_group_entity_group_reference
# WITHOUT also supplying secured_group_category_references — the path that
# pre-fix was rejected at plan time (schema validation) AND, if you tried to
# satisfy the schema by also setting category_references, the v4.2 API would
# have rejected with MIC-30142.
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
        # Cluster requires the rule to declare what protocols it allows
        # (MIC-30124). Use is_all_protocol_allowed to keep the smoke test
        # focused on the entity_group_reference fix — the protocol surface
        # is orthogonal to bug #1183.
        is_all_protocol_allowed = true
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
