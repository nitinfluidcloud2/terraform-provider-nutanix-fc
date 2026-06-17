# Smoke test — ICMP wildcard fix (`2.4.3-beta1-fc3`)

Verifies the `icmp_services { is_all_allowed = true }` HCL applies cleanly
on PC 7.5 / FNS v4.2 with the fc3 build. Pre-fix the provider serialized
the schema-default `type=0` / `code=0` into the API payload alongside
`is_all_allowed=true` and the backend rejected with:

| Resource | API error |
|---|---|
| `nutanix_service_groups_v2` | **MIC-30302** — "ICMP is set to allow all but also contains specific type/code" |
| `nutanix_network_security_policy_v2` (inline rule) | **MIC-30113** — same root cause, policy-side surface |

The fix gates the type/code serialization on `!is_all_allowed`. See
`nutanix/services/networkingv2/resource_nutanix_service_groups_v2.go:377`
(`expandIcmpTypeCodeSpec`).

## Files

| File | Purpose |
|---|---|
| `main.tf` | Two resources — standalone `nutanix_service_groups_v2` AND a `nutanix_network_security_policy_v2` whose rule carries inline `icmp_services { is_all_allowed = true }` |

## Quick run

```bash
cd /Users/nitinmore/go/nutanixfork/terraform-provider-nutanix-fc/smoke-test/icmp-wildcard

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

- `terraform apply` completes with no `MIC-30302` or `MIC-30113` error
- `terraform destroy` removes both resources cleanly
- After destroy, the category `fc_icmp_smoke_tier` parent bucket is also gone (uses the fc2 orphan-parent cleanup)

## Negative control

To prove the smoke test would catch a regression, temporarily switch the
provider version pin in `main.tf` to upstream `2.4.3-beta1` (drop `-fc3`),
re-run `terraform init -reconfigure && terraform apply`. Expected: `apply`
fails on the service group with **MIC-30302**.

## Why the gate is on `is_all_allowed`, not `!= 0`

The original proposed upstream fix used `!= 0` guards on `code` and
`type`. That approach is narrower: it cannot represent the specific case
`type = 0, code = 0` (ICMP Echo Reply) with `is_all_allowed = false`. The
fork uses an `is_all_allowed=true` gate instead — when the user opts into
the wildcard, type/code are unconditionally dropped; otherwise the
historical behavior is preserved. This avoids the upstream tradeoff
entirely.

## Unit-test coverage

Companion unit test at
`nutanix/services/networkingv2/resource_nutanix_service_groups_v2_icmp_test.go`
covers:

- `is_all_allowed=true` omits type/code even when the schema map fills them with 0
- `is_all_allowed=false, type=8, code=0` preserves both (ICMP Echo Request with code 0 is legitimate)
- `is_all_allowed` unset (default false) keeps historical behavior
- Empty input returns nil
- Multiple `icmp_services` blocks handled independently
