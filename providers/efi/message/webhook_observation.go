package message

import "github.com/dakasa-yggdrasil/integration-efi/providers/efi/adapter/capabilities"

// webhookObservationOutput preserves the public raw provider observation while
// retaining the items/cursor envelope existing SDK consumers already receive.
// The items alias cannot establish page completeness; callers must validate the
// original provider parameters and every original record.
func webhookObservationOutput(value any) any {
	return capabilities.WebhookObservationOutput(value)
}
