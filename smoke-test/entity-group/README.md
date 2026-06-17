# Smoke test — entity_group_reference mutex fix (`2.4.3-beta1-fc4`)

Verifies the `application_rule_spec.secured_group_entity_group_reference`
path now works without supplying `secured_group_category_references` on
the same rule.

## The bug pre-fix

The provider schema marked `application_rule_spec.secured_group_category_references`
as **`Required: true`**, but the v4.2 microseg API treats
`secured_group_category_references` and `secured_group_entity_group_reference`
as **mutually exclusive** (MIC-30142). The two requirements were
unsatisfiable: any HCL using `entity_group_reference` either failed
plan-time validation (no category refs) or failed apply (mutex error).

The fork patches the schema to `Optional` on both, matching the v4.2 API
contract. The expand function already handles either side being absent —
no Create-time change needed.

See:
- Patched file: `nutanix/services/networkingv2/resource_nutanix_network_security_policies_v2.go` (line 110 region)
- Unit tests: `nutanix/services/networkingv2/resource_nutanix_network_security_policies_v2_entity_group_test.go` (4 tests)
- Upstream issue: <https://github.com/nutanix/terraform-provider-nutanix/issues/1183>

## Files

| File | Purpose |
|---|---|
| `main.tf` | 2 categories + 1 entity_group + 1 policy whose rule uses ONLY `entity_group_reference` |

## Quick run

```bash
cd /Users/nitinmore/go/nutanixfork/terraform-provider-nutanix-fc/smoke-test/entity-group

# point Terraform at the local mirror (one level up)
export TF_CLI_CONFIG_FILE="$(realpath ../terraformrc)"

# Nutanix creds
export TF_VAR_nutanix_user='admin2'
export TF_VAR_nutanix_password='...'
export TF_VAR_nutanix_endpoint='cluster-1919.nutanix.ovh.us'

terraform init -reconfigure
terraform apply -auto-approve     # MUST succeed end-to-end
terraform destroy -auto-approve
```

## Success criteria

- `terraform plan` does NOT fail with "secured_group_category_references is required"
- `terraform apply` does NOT fail with `MIC-30142`
- The policy is created on the cluster with the entity_group_reference live
- `terraform destroy` removes everything (including the category parent buckets thanks to fc2's orphan-parent cleanup)

## Negative control

Pin `main.tf` to upstream `2.4.3-beta1` (drop `-fc4`), re-run `terraform init -reconfigure && terraform plan`. Expected: plan fails at validation:

```
Error: Missing required argument
  The argument "secured_group_category_references" is required, but no
  definition was found.
```

## Unit-test coverage

Companion unit tests at
`nutanix/services/networkingv2/resource_nutanix_network_security_policies_v2_entity_group_test.go`
cover:

- entity_group_reference alone → resulting ApplicationRuleSpec has it set, no category refs
- category_references alone → preserved verbatim, entity_group_reference nil
- Both set → both passed through (provider does NOT enforce mutex client-side; cluster does)
- Schema check that `secured_group_category_references` is `Optional`, not `Required`
