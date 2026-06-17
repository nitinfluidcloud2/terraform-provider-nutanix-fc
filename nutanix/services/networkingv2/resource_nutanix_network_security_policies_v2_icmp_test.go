package networkingv2

import (
	"testing"

	import1 "github.com/nutanix/ntnx-api-golang-clients/microseg-go-client/v4/models/microseg/v4/config"
)

// TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationICMPWildcard
// pins the inline-on-rule path used by nutanix_network_security_policy_v2's
// application_rule_spec.icmp_services. This is the MIC-30113 surface — the
// same backend that rejects MIC-30302 on standalone service groups.
//
// The expand function is shared with nutanix_service_groups_v2 via
// expandIcmpTypeCodeSpec, so the fix landed there too — but a dedicated
// policy-path test guards against a future refactor that forks the helper.
func TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationICMPWildcard(t *testing.T) {
	// Build the HCL-equivalent map that Terraform's schema decoder would hand
	// the expand function for:
	//   rules {
	//     spec {
	//       application_rule_spec {
	//         icmp_services { is_all_allowed = true }
	//       }
	//     }
	//   }
	in := []interface{}{
		map[string]interface{}{
			"application_rule_spec": []interface{}{
				map[string]interface{}{
					"icmp_services": []interface{}{
						map[string]interface{}{
							"is_all_allowed": true,
							"code":           0, // Terraform schema fills these with 0;
							"type":           0, // the fix must DROP them on wildcard.
						},
					},
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
	if len(app.IcmpServices) != 1 {
		t.Fatalf("expected 1 IcmpServices entry, got %d", len(app.IcmpServices))
	}
	icmp := app.IcmpServices[0]
	if icmp.IsAllAllowed == nil || !*icmp.IsAllAllowed {
		t.Fatalf("expected IsAllAllowed=true, got %v", icmp.IsAllAllowed)
	}
	if icmp.Type != nil {
		t.Fatalf("policy-rule path leaked Type=%d into payload (MIC-30113 regression)", *icmp.Type)
	}
	if icmp.Code != nil {
		t.Fatalf("policy-rule path leaked Code=%d into payload (MIC-30113 regression)", *icmp.Code)
	}
}

// TestExpandOneOfNetworkSecurityPolicyRuleSpec_IntraICMPWildcard
// pins the same fix on the INTRA_GROUP rule spec — a separate code path
// in the same expand function. Without this test a refactor that
// inadvertently moves the intra-spec path off the shared helper would
// reintroduce the bug for intra-group rules only.
func TestExpandOneOfNetworkSecurityPolicyRuleSpec_IntraICMPWildcard(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{
			"intra_entity_group_rule_spec": []interface{}{
				map[string]interface{}{
					"secured_group_action": "DENY",
					"icmp_services": []interface{}{
						map[string]interface{}{
							"is_all_allowed": true,
							"code":           0,
							"type":           0,
						},
					},
				},
			},
		},
	}

	got := expandOneOfNetworkSecurityPolicyRuleSpec(in)
	if got == nil {
		t.Fatal("expand returned nil")
	}
	intra, ok := got.GetValue().(import1.IntraEntityGroupRuleSpec)
	if !ok {
		t.Fatalf("expected IntraEntityGroupRuleSpec, got %T", got.GetValue())
	}
	if len(intra.IcmpServices) != 1 {
		t.Fatalf("expected 1 IcmpServices entry, got %d", len(intra.IcmpServices))
	}
	icmp := intra.IcmpServices[0]
	if icmp.IsAllAllowed == nil || !*icmp.IsAllAllowed {
		t.Fatalf("expected IsAllAllowed=true, got %v", icmp.IsAllAllowed)
	}
	if icmp.Type != nil {
		t.Fatalf("intra-group path leaked Type=%d into payload (MIC-30113 regression)", *icmp.Type)
	}
	if icmp.Code != nil {
		t.Fatalf("intra-group path leaked Code=%d into payload (MIC-30113 regression)", *icmp.Code)
	}
}

// TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationICMPSpecificTypeCode
// negative-control: when is_all_allowed=false and the user DID specify
// type/code, the policy-rule path must preserve them. This protects
// against an over-eager fix that drops type/code unconditionally.
func TestExpandOneOfNetworkSecurityPolicyRuleSpec_ApplicationICMPSpecificTypeCode(t *testing.T) {
	in := []interface{}{
		map[string]interface{}{
			"application_rule_spec": []interface{}{
				map[string]interface{}{
					"icmp_services": []interface{}{
						map[string]interface{}{
							"is_all_allowed": false,
							"type":           8, // Echo Request
							"code":           0,
						},
					},
				},
			},
		},
	}

	got := expandOneOfNetworkSecurityPolicyRuleSpec(in)
	app, ok := got.GetValue().(import1.ApplicationRuleSpec)
	if !ok {
		t.Fatalf("expected ApplicationRuleSpec, got %T", got.GetValue())
	}
	icmp := app.IcmpServices[0]
	if icmp.Type == nil || *icmp.Type != 8 {
		t.Fatalf("expected Type=8 preserved, got %v", icmp.Type)
	}
	if icmp.Code == nil || *icmp.Code != 0 {
		t.Fatalf("expected Code=0 (intentional) preserved, got %v", icmp.Code)
	}
	if icmp.IsAllAllowed == nil || *icmp.IsAllAllowed {
		t.Fatalf("expected IsAllAllowed=false, got %v", icmp.IsAllAllowed)
	}
}
