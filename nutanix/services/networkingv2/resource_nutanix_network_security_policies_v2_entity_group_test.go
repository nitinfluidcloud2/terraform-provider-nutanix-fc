package networkingv2

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	import1 "github.com/nutanix/ntnx-api-golang-clients/microseg-go-client/v4/models/microseg/v4/config"
)

// Pre-fix behavior: the application_rule_spec schema declared
// secured_group_category_references as Required:true. Users who wanted
// to reference a dynamic nutanix_entity_group_v2 via
// secured_group_entity_group_reference had to ALSO supply
// secured_group_category_references — but the v4.2 API treats them as
// mutually exclusive (MIC-30142). The two requirements were unsatisfiable.
//
// The fix makes secured_group_category_references Optional. The expand
// function already gracefully handles either side being unset
// (len(...) > 0 guards). These tests pin the new schema behavior so a
// future refactor that re-tightens the schema gets caught immediately.

// TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationEntityGroupOnly
// is the new path the fork unblocks: an application_rule_spec that uses
// secured_group_entity_group_reference without supplying
// secured_group_category_references.
func TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationEntityGroupOnly(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{
			"application_rule_spec": []interface{}{
				map[string]interface{}{
					// No secured_group_category_references — the v4.2 API
					// rejects payloads that carry BOTH, so the fork must NOT
					// require the user to supply it when entity_group_reference
					// is the chosen reference path.
					"secured_group_entity_group_reference": "eg-extid-1234-5678",
					"src_allow_spec":                       "ALL",
				},
			},
		},
	}

	got := expandOneOfNetworkSecurityPolicyRuleSpec(in)
	if got == nil {
		t.Fatal("expand returned nil")
	}
	app, ok := got.GetValue().(import1.ApplicationRuleSpec)
	if !ok {
		t.Fatalf("expected ApplicationRuleSpec, got %T", got.GetValue())
	}
	if app.SecuredGroupEntityGroupReference == nil || *app.SecuredGroupEntityGroupReference != "eg-extid-1234-5678" {
		t.Fatalf("expected SecuredGroupEntityGroupReference=eg-extid-1234-5678, got %v", app.SecuredGroupEntityGroupReference)
	}
	if len(app.SecuredGroupCategoryReferences) != 0 {
		t.Fatalf("expected NO category references (MIC-30142 mutex), got %v", app.SecuredGroupCategoryReferences)
	}
}

// TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationCategoryOnly
// is the historical path — secured_group_category_references supplied,
// entity_group_reference omitted. The fix must NOT regress this.
func TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationCategoryOnly(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{
			"application_rule_spec": []interface{}{
				map[string]interface{}{
					"secured_group_category_references": []interface{}{"cat-1", "cat-2"},
					"src_allow_spec":                    "ALL",
				},
			},
		},
	}

	got := expandOneOfNetworkSecurityPolicyRuleSpec(in)
	app, ok := got.GetValue().(import1.ApplicationRuleSpec)
	if !ok {
		t.Fatalf("expected ApplicationRuleSpec, got %T", got.GetValue())
	}
	if len(app.SecuredGroupCategoryReferences) != 2 {
		t.Fatalf("expected 2 category refs, got %d", len(app.SecuredGroupCategoryReferences))
	}
	if app.SecuredGroupEntityGroupReference != nil {
		t.Fatalf("expected nil EntityGroupReference, got %v", app.SecuredGroupEntityGroupReference)
	}
}

// TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationBothSetIsCallerError
// asserts that when the operator supplies BOTH references, the provider
// passes them through verbatim — leaving it to the v4.2 backend to return
// MIC-30142. The provider does NOT enforce the mutex client-side; this is
// the existing behavior and the fix preserves it. The test pins that we
// did not accidentally add client-side rejection that would now block
// legitimate fix-upgrade paths from old tfstate.
func TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationBothSetIsCallerError(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{
			"application_rule_spec": []interface{}{
				map[string]interface{}{
					"secured_group_category_references":    []interface{}{"cat-1"},
					"secured_group_entity_group_reference": "eg-extid-9999",
					"src_allow_spec":                       "ALL",
				},
			},
		},
	}

	got := expandOneOfNetworkSecurityPolicyRuleSpec(in)
	app, ok := got.GetValue().(import1.ApplicationRuleSpec)
	if !ok {
		t.Fatalf("expected ApplicationRuleSpec, got %T", got.GetValue())
	}
	if len(app.SecuredGroupCategoryReferences) != 1 {
		t.Fatalf("expected category refs preserved, got %d", len(app.SecuredGroupCategoryReferences))
	}
	if app.SecuredGroupEntityGroupReference == nil {
		t.Fatalf("expected entity_group_reference preserved, got nil")
	}
}

// TestApplicationRuleSpecSchemaCategoryReferencesIsOptional is the schema-
// level pin: the application_rule_spec sub-schema must declare
// secured_group_category_references as Optional, not Required. Without
// this, any user who tries to drive the rule via entity_group_reference
// alone gets a plan-time validation error before the v4.2 mutex ever has
// a chance to fire.
func TestApplicationRuleSpecSchemaCategoryReferencesIsOptional(t *testing.T) {
	resource := ResourceNutanixNetworkSecurityPolicyV2()
	rulesSchema := resource.Schema["rules"]
	if rulesSchema == nil {
		t.Fatal("rules schema not present")
	}
	rulesElem, ok := rulesSchema.Elem.(*schema.Resource)
	if !ok {
		t.Fatalf("rules.Elem unexpected type %T", rulesSchema.Elem)
	}
	specSchema := rulesElem.Schema["spec"]
	specElem := specSchema.Elem.(*schema.Resource)
	appRuleSchema := specElem.Schema["application_rule_spec"]
	appRuleElem := appRuleSchema.Elem.(*schema.Resource)
	catRefs := appRuleElem.Schema["secured_group_category_references"]
	if catRefs == nil {
		t.Fatal("secured_group_category_references not in application_rule_spec schema")
	}
	if catRefs.Required {
		t.Fatal("secured_group_category_references must NOT be Required — that's the bug #1183 the fork is fixing")
	}
	if !catRefs.Optional {
		t.Fatal("secured_group_category_references must be Optional so entity_group_reference can be used alone")
	}
}
