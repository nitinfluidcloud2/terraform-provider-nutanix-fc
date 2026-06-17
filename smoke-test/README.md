# Smoke test — forked Nutanix provider `2.4.3-beta1-fc2` (orphan-parent cleanup)

Verifies the orphan-parent-bucket cleanup is live in the built provider binary.
Resolves the fork from the local **filesystem mirror** without ever contacting
the public registry.

This smoke test calls the live Nutanix API (Create + Delete) to exercise the
new parent-bucket cleanup helpers. You need a reachable Prism Central with
credentials that can manage categories.

## Files

| File | Purpose |
|---|---|
| `terraformrc` | CLI config: points Terraform at `../dist/fs-mirror` and excludes nutanix from the registry |
| `main.tf` | A single `nutanix_category_v2` resource that would orphan its parent bucket pre-fix |
| `verify-no-orphan.sh` | Post-destroy check: queries `v4.0.a1` for the parent bucket and exits non-zero if it's still there |

## Quick run

```bash
cd /Users/nitinmore/go/nutanixfork/terraform-provider-nutanix-fc/smoke-test

# point Terraform at the local mirror
export TF_CLI_CONFIG_FILE="$(pwd)/terraformrc"

# Nutanix creds (also picked up by verify-no-orphan.sh)
export TF_VAR_nutanix_user='admin2'
export TF_VAR_nutanix_password='...'
export TF_VAR_nutanix_endpoint='cluster-1919.nutanix.ovh.us'

# same creds in the form verify-no-orphan.sh expects
export NUTANIX_USERNAME="$TF_VAR_nutanix_user"
export NUTANIX_PASSWORD="$TF_VAR_nutanix_password"
export NUTANIX_ENDPOINT="$TF_VAR_nutanix_endpoint:9440"

# clean any prior orphan from a previous run (best-effort)
./verify-no-orphan.sh || true

# baseline cycle
terraform init -reconfigure
terraform apply -auto-approve
terraform destroy -auto-approve

# THE check
./verify-no-orphan.sh
# expected: ✅ no orphan parent found — cleanup worked.
```

## What success looks like

```
>> looking for orphan parent bucket with name = 'fc_orphan_smoke' (post-destroy)
>> ✅ no orphan parent found — cleanup worked.
```

## What failure looks like

```
>> looking for orphan parent bucket with name = 'fc_orphan_smoke' (post-destroy)
>> ❌ orphan parent(s) detected:
   extId=d52904d2-869e-7de3-f8d6-51fad1396496  name=fc_orphan_smoke  child_count=0
```

If you see the failure: the patched provider didn't manage to discover the
parent at Create time, or the Delete-time emptiness check fired wrong. Check
`TF_LOG=DEBUG terraform destroy` and look for `[DEBUG] empty parent bucket … deleted`
or `[WARN] could not verify parent bucket … is empty`.

## How the fix works

The Nutanix v4 backend stores each category as a two-level tree:

- A parent **key** bucket — extId K, name=`<key>`, no value, parentExtId=null
- One or more child **value** leaves under that bucket — extId C, name=`<value>`, parentExtId=K

The GA `Category` model (used by the SDK's `CreateCategory` / `GetCategoryById` /
`DeleteCategoryById`) does NOT expose `parentExtId` or `childCategories`. Before
this fork, the provider:

1. Created the category → backend created both K and C, returned only C
2. Stored C in state as `id`
3. On destroy → DELETEd C; **K leaked as an empty orphan**

The fork patches `resource_nutanix_categories_v2.go` to:

1. **Create**: after `CreateCategory`, list `/api/prism/v4.0.a1/config/categories?$filter=name eq '<key>' and parentExtId eq null` to find the parent bucket extId, save to a new computed `parent_ext_id` schema attribute. (v4.0.a1 is the same endpoint the Prism UI uses to render the category page.)
2. **Read**: if `parent_ext_id` is missing (e.g. after `terraform import`), re-discover it.
3. **Delete**: after the existing child DELETE, GET the parent with `$expand=childCategories`; if empty, DELETE the parent too. The Delete call goes through the GA SDK (which negotiates to v4.2 on PC 7.5 — same as the UI's delete).

All discovery/emptiness checks reuse the provider's existing `ApiClient` (no new SDK, no `go.mod` change). The only thing different from a "stock" GA call is the URI string fed to `CallApi`.

## Negative control (skip-this-fix safety)

If you want to confirm the test would CATCH a regression, temporarily run
`terraform apply` against the upstream `2.4.3-beta1` provider (point
`terraformrc` away from `dist/fs-mirror`), then `terraform destroy`, then
`./verify-no-orphan.sh` — it should report the orphan.

## Notes

- The `value = "fc_orphan_smoke"` keeps the leaf and bucket names identical, which makes the orphan obvious in the UI even without `verify-no-orphan.sh`.
- Two TF runs back-to-back share the same parent bucket — the second `apply` will reuse the bucket created by the first. That's by design and exercises the "siblings remain → don't delete bucket" branch when only one of the resources is destroyed.
