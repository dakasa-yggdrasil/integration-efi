package adapter

import (
	"testing"
)

// TestLegacyOperationAliases_Coverage locks the v1.x → v2.0.0 alias
// table. If any of these mappings change, callers still emitting the
// pre-convention name break without warning.
func TestLegacyOperationAliases_Coverage(t *testing.T) {
	expect := map[string]string{
		"create_charge":               OperationEnsureCharge,
		"create_due_charge":           OperationEnsureDueCharge,
		"get_charge_status":           OperationObserveCharges,
		"get_statement":               OperationObserveCharges,
		"register_webhook_endpoint":   OperationEnsureWebhookSubscription,
		"unregister_webhook_endpoint": OperationDestroyWebhookSubscription,
	}
	for legacy, want := range expect {
		got, ok := LegacyOperationAliases[legacy]
		if !ok {
			t.Errorf("missing legacy alias for %q", legacy)
			continue
		}
		if got != want {
			t.Errorf("legacy alias %q → %q, want %q", legacy, got, want)
		}
	}
	// Reverse direction: no v2.0.0 canonical name should leak as a
	// "legacy" alias (that would create circular aliases).
	for legacy := range LegacyOperationAliases {
		if SupportsExecuteCapabilityCanonicalOnly(legacy) {
			t.Errorf("legacy alias %q must NOT match a canonical operation in SupportedExecuteOperations", legacy)
		}
	}
}

// SupportsExecuteCapabilityCanonicalOnly is a test helper that mirrors
// SupportsExecuteCapability but skips the legacy fallback — used to
// assert legacy names don't accidentally collide with canonical names.
func SupportsExecuteCapabilityCanonicalOnly(value string) bool {
	for _, supported := range SupportedExecuteOperations {
		if value == supported {
			return true
		}
	}
	return false
}

// TestCanonicalOperationFor_PassesThroughCanonicalNames asserts the
// shim does not loop / re-warn when callers already use the v2.0.0
// canonical name.
func TestCanonicalOperationFor_PassesThroughCanonicalNames(t *testing.T) {
	for _, name := range []string{
		OperationEnsureCharge,
		OperationEnsureDueCharge,
		OperationObserveCharges,
		OperationDestroyCharge,
		OperationEnsureWebhookSubscription,
		OperationObserveWebhookSubscriptions,
		OperationDestroyWebhookSubscription,
	} {
		got, legacy := CanonicalOperationFor(name)
		if got != name {
			t.Errorf("canonical %q got rewritten to %q", name, got)
		}
		if legacy {
			t.Errorf("canonical %q misclassified as legacy", name)
		}
	}
}

// TestCanonicalOperationFor_LegacyAliasesResolve asserts every entry
// in LegacyOperationAliases routes to its canonical replacement when
// fed back through CanonicalOperationFor.
func TestCanonicalOperationFor_LegacyAliasesResolve(t *testing.T) {
	for legacy, want := range LegacyOperationAliases {
		got, isLegacy := CanonicalOperationFor(legacy)
		if !isLegacy {
			t.Errorf("expected %q to be flagged as legacy", legacy)
		}
		if got != want {
			t.Errorf("CanonicalOperationFor(%q) = %q, want %q", legacy, got, want)
		}
	}
}

// TestSupportsExecuteCapability_AcceptsLegacy asserts the message
// handler's gate still admits v1.x names — without this the
// SupportsExecuteCapability check in providers/efi/message/execute.go
// would reject the legacy operation BEFORE the shim could remap it.
func TestSupportsExecuteCapability_AcceptsLegacy(t *testing.T) {
	for legacy := range LegacyOperationAliases {
		if !SupportsExecuteCapability(legacy) {
			t.Errorf("SupportsExecuteCapability(%q) = false, want true (compat shim)", legacy)
		}
	}
}
