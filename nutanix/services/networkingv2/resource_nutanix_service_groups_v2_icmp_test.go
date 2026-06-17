package networkingv2

import (
	"testing"
)

// TestExpandIcmpTypeCodeSpec covers the orphan-type/code regression fixed
// in the fork: when the HCL specifies only is_all_allowed=true, the
// provider must NOT serialize the schema-default type=0 / code=0 fields
// into the API payload. Pre-fix the backend rejected such payloads with
// MIC-30302 (service groups) / MIC-30113 (policies).
func TestExpandIcmpTypeCodeSpec(t *testing.T) {
	t.Run("is_all_allowed=true omits type and code even when map contains them", func(t *testing.T) {
		// This map is what Terraform's schema-driven d.Get hands us when the
		// HCL is:
		//   icmp_services { is_all_allowed = true }
		// Note that "code" and "type" are present in the map with the int
		// zero value because they are schema-declared. The fix must not
		// echo them back into the API payload.
		input := []interface{}{
			map[string]interface{}{
				"is_all_allowed": true,
				"code":           0,
				"type":           0,
			},
		}
		got := expandIcmpTypeCodeSpec(input)
		if len(got) != 1 {
			t.Fatalf("expected 1 spec, got %d", len(got))
		}
		spec := got[0]
		if spec.IsAllAllowed == nil || !*spec.IsAllAllowed {
			t.Fatalf("expected IsAllAllowed=true, got %v", spec.IsAllAllowed)
		}
		if spec.Code != nil {
			t.Fatalf("expected Code to be omitted (nil), got *Code=%d", *spec.Code)
		}
		if spec.Type != nil {
			t.Fatalf("expected Type to be omitted (nil), got *Type=%d", *spec.Type)
		}
	})

	t.Run("is_all_allowed=false keeps explicit type and code", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"is_all_allowed": false,
				"type":           8, // Echo Request
				"code":           0,
			},
		}
		got := expandIcmpTypeCodeSpec(input)
		if len(got) != 1 {
			t.Fatalf("expected 1 spec, got %d", len(got))
		}
		spec := got[0]
		if spec.IsAllAllowed == nil || *spec.IsAllAllowed {
			t.Fatalf("expected IsAllAllowed=false, got %v", spec.IsAllAllowed)
		}
		if spec.Type == nil || *spec.Type != 8 {
			t.Fatalf("expected Type=8, got %v", spec.Type)
		}
		if spec.Code == nil || *spec.Code != 0 {
			t.Fatalf("expected Code=0 (intentionally set), got %v", spec.Code)
		}
	})

	t.Run("is_all_allowed unset (default false) preserves prior behavior", func(t *testing.T) {
		// Terraform fills bool defaults as false when the user omits the
		// field. Without is_all_allowed, the existing semantics must hold:
		// type/code serialize as written.
		input := []interface{}{
			map[string]interface{}{
				"is_all_allowed": false,
				"type":           3,
				"code":           1,
			},
		}
		got := expandIcmpTypeCodeSpec(input)
		spec := got[0]
		if spec.Type == nil || *spec.Type != 3 {
			t.Fatalf("expected Type=3, got %v", spec.Type)
		}
		if spec.Code == nil || *spec.Code != 1 {
			t.Fatalf("expected Code=1, got %v", spec.Code)
		}
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		if got := expandIcmpTypeCodeSpec(nil); got != nil {
			t.Fatalf("expected nil for empty input, got %#v", got)
		}
		if got := expandIcmpTypeCodeSpec([]interface{}{}); got != nil {
			t.Fatalf("expected nil for empty input slice, got %#v", got)
		}
	})

	t.Run("multiple icmp_services blocks handled independently", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{"is_all_allowed": true, "type": 0, "code": 0},
			map[string]interface{}{"is_all_allowed": false, "type": 8, "code": 0},
		}
		got := expandIcmpTypeCodeSpec(input)
		if len(got) != 2 {
			t.Fatalf("expected 2 specs, got %d", len(got))
		}
		if got[0].Type != nil || got[0].Code != nil {
			t.Fatalf("first spec: is_all_allowed=true should omit type/code, got type=%v code=%v", got[0].Type, got[0].Code)
		}
		if got[1].Type == nil || *got[1].Type != 8 {
			t.Fatalf("second spec: expected Type=8, got %v", got[1].Type)
		}
	})

	t.Run("is_all_allowed=true with nonzero type and code still drops them", func(t *testing.T) {
		// Defensive: even if the user (incorrectly) supplies type/code
		// alongside is_all_allowed=true, the wildcard intent wins and the
		// API payload must NOT carry the conflicting fields.
		input := []interface{}{
			map[string]interface{}{
				"is_all_allowed": true,
				"type":           8, // would normally be Echo Request
				"code":           5,
			},
		}
		got := expandIcmpTypeCodeSpec(input)
		spec := got[0]
		if spec.IsAllAllowed == nil || !*spec.IsAllAllowed {
			t.Fatalf("expected IsAllAllowed=true, got %v", spec.IsAllAllowed)
		}
		if spec.Type != nil {
			t.Fatalf("is_all_allowed=true must drop Type even when user supplied %d, got *Type=%d", 8, *spec.Type)
		}
		if spec.Code != nil {
			t.Fatalf("is_all_allowed=true must drop Code even when user supplied %d, got *Code=%d", 5, *spec.Code)
		}
	})

	t.Run("is_all_allowed=true with only type supplied still drops it", func(t *testing.T) {
		// HCL author writes:
		//   icmp_services { is_all_allowed = true, type = 8 }
		// The fix must NOT echo `type` back into the payload.
		input := []interface{}{
			map[string]interface{}{
				"is_all_allowed": true,
				"type":           8,
				"code":           0,
			},
		}
		spec := expandIcmpTypeCodeSpec(input)[0]
		if spec.Type != nil {
			t.Fatalf("expected Type omitted, got *Type=%d", *spec.Type)
		}
	})

	t.Run("is_all_allowed=true with only code supplied still drops it", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"is_all_allowed": true,
				"type":           0,
				"code":           3,
			},
		}
		spec := expandIcmpTypeCodeSpec(input)[0]
		if spec.Code != nil {
			t.Fatalf("expected Code omitted, got *Code=%d", *spec.Code)
		}
	})

	t.Run("is_all_allowed missing-from-map falls back to NOT wildcard", func(t *testing.T) {
		// When Terraform omits is_all_allowed from the map entirely (rare but
		// possible — e.g. provider acceptance tests that hand-build the map),
		// the helper must NOT default to wildcard semantics. type/code must
		// flow through as written so existing user expectations hold.
		input := []interface{}{
			map[string]interface{}{
				"type": 11,
				"code": 0,
			},
		}
		spec := expandIcmpTypeCodeSpec(input)[0]
		if spec.IsAllAllowed != nil {
			t.Fatalf("expected IsAllAllowed nil when not in map, got %v", spec.IsAllAllowed)
		}
		if spec.Type == nil || *spec.Type != 11 {
			t.Fatalf("expected Type=11 preserved, got %v", spec.Type)
		}
		if spec.Code == nil || *spec.Code != 0 {
			t.Fatalf("expected Code=0 preserved, got %v", spec.Code)
		}
	})

	t.Run("mixed wildcard and specific in one expansion", func(t *testing.T) {
		// A more aggressive variant of the existing multi-block test:
		// 3 entries covering wildcard / specific / second-wildcard-with-pollution.
		input := []interface{}{
			map[string]interface{}{"is_all_allowed": true, "type": 0, "code": 0},
			map[string]interface{}{"is_all_allowed": false, "type": 3, "code": 1},
			map[string]interface{}{"is_all_allowed": true, "type": 8, "code": 0}, // pollution attempt
		}
		got := expandIcmpTypeCodeSpec(input)
		if len(got) != 3 {
			t.Fatalf("expected 3 specs, got %d", len(got))
		}
		// entry 0 — clean wildcard
		if got[0].Type != nil || got[0].Code != nil || got[0].IsAllAllowed == nil || !*got[0].IsAllAllowed {
			t.Fatalf("entry 0 should be clean wildcard, got %+v", got[0])
		}
		// entry 1 — specific type 3 code 1
		if got[1].Type == nil || *got[1].Type != 3 || got[1].Code == nil || *got[1].Code != 1 {
			t.Fatalf("entry 1 should preserve type=3 code=1, got type=%v code=%v", got[1].Type, got[1].Code)
		}
		// entry 2 — wildcard with type=8 supplied must still be clean
		if got[2].Type != nil || got[2].Code != nil {
			t.Fatalf("entry 2 should be clean wildcard despite user-supplied type=8, got type=%v code=%v", got[2].Type, got[2].Code)
		}
	})
}
