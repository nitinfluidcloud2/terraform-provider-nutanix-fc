###############################################################################
# Smoke test for terraform-provider-nutanix 2.4.3-beta1-fc2
#
# Verifies the orphan-parent-bucket cleanup is live: a category created and
# then destroyed should leave NO trace on the cluster — neither the child
# leaf nor the parent "key" bucket.
#
# Requires a reachable Prism Central. Unlike the schema-only smoke tests, this
# one calls the live API to exercise Create + Delete and the parent-bucket
# cleanup helpers.
#
# Env vars (do NOT hardcode passwords):
#   TF_VAR_nutanix_user
#   TF_VAR_nutanix_password
#   TF_VAR_nutanix_endpoint
#
# Run:
#   export TF_CLI_CONFIG_FILE=$(pwd)/terraformrc
#   terraform init
#   terraform apply -auto-approve
#   terraform destroy -auto-approve
#   ./verify-no-orphan.sh    # see README
###############################################################################

terraform {
  required_providers {
    nutanix = {
      source  = "nutanix/nutanix"
      version = "2.4.3-beta1-fc2"
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

# A category whose key+value are unique enough not to collide with anything
# else on the cluster — easy to find/verify in the UI after apply/destroy.
resource "nutanix_category_v2" "orphan_smoke" {
  key         = "fc_orphan_smoke"
  value       = "fc_orphan_smoke"
  description = "Smoke test for orphan-parent-bucket cleanup (fc2 build)"
}

output "child_ext_id" {
  value = nutanix_category_v2.orphan_smoke.id
}

output "parent_ext_id" {
  value = nutanix_category_v2.orphan_smoke.parent_ext_id
}
