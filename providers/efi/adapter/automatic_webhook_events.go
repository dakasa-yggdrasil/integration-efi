package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil-sdk-go/sdk/events"

	"github.com/dakasa-yggdrasil/integration-efi/family/contract"
)

// automaticWebhookEventSink emits the §6.5 mutation event of
// ensure_automatic_webhook. The resource is served by the legacy Execute
// switch, not by an SDK Reconciler, because RegisterReconciler always
// installs a destroy_ operation and this resource deliberately has none.
// The SDK emits only for Reconciler-backed operations, so this sink does
// the same work explicitly with the same emitter and the same best-effort
// rule: an emit failure is logged as a WARN and never turns a proven
// provider mutation into a failed call.
type automaticWebhookEventSink struct {
	emitter    events.Emitter
	instanceID string
	warn       func(format string, args ...any)
}

var (
	automaticWebhookEventsMu sync.RWMutex
	automaticWebhookEvents   = automaticWebhookEventSink{
		emitter: &events.NoopEmitter{},
		warn: func(format string, args ...any) {
			log.Printf("WARN "+format, args...)
		},
	}
)

// configureAutomaticWebhookEvents installs the emitter used for
// efi.automatic_webhook.ensured. WireReconcilers calls it with the same
// emitter and fallback instance it gives the SDK reconcilers.
func configureAutomaticWebhookEvents(emitter events.Emitter, instanceID string) {
	automaticWebhookEventsMu.Lock()
	defer automaticWebhookEventsMu.Unlock()
	if emitter == nil {
		emitter = &events.NoopEmitter{}
	}
	automaticWebhookEvents.emitter = emitter
	automaticWebhookEvents.instanceID = strings.TrimSpace(instanceID)
}

func currentAutomaticWebhookEvents() automaticWebhookEventSink {
	automaticWebhookEventsMu.RLock()
	defer automaticWebhookEventsMu.RUnlock()
	return automaticWebhookEvents
}

// emitEnsured posts efi.automatic_webhook.ensured after a successful
// ensure_automatic_webhook. resource_id is the EFI endpoint name
// (webhookrec or webhookcobr); instance_id is the request's integration
// instance name, falling back to the adapter's configured instance.
func (s automaticWebhookEventSink) emitEnsured(ctx context.Context, req contract.AdapterExecuteIntegrationRequest, observed map[string]any) {
	resourceID, _ := observed["resource_id"].(string)
	instanceID := strings.TrimSpace(req.Integration.Instance.Name)
	if instanceID == "" {
		instanceID = s.instanceID
	}
	eventType := events.BuildEventType(Provider, ResourceAutomaticWebhook, events.VerbEnsured)
	body, err := json.Marshal(observed)
	if err != nil {
		s.warn("integration-efi: emit %q skipped: marshal observed state: %v", eventType, err)
		return
	}
	idempotency, _ := req.Metadata["idempotency"].(string)
	if strings.TrimSpace(idempotency) == "" {
		idempotency = synthesizeAutomaticWebhookIdempotency(resourceID)
	}
	event := events.MutationEvent{
		EventType:   eventType,
		Provider:    Provider,
		Resource:    ResourceAutomaticWebhook,
		Verb:        events.VerbEnsured,
		ResourceID:  resourceID,
		InstanceID:  instanceID,
		Idempotency: idempotency,
		Observed:    body,
	}
	if err := s.emitter.Emit(ctx, event); err != nil {
		s.warn("integration-efi: emit %q for %s failed (best-effort, the provider state was proven by readback): %v", eventType, resourceID, err)
	}
}

// synthesizeAutomaticWebhookIdempotency mirrors the SDK's key shape for
// envelopes without a caller idempotency key:
// <provider>.<resource>.<verb>.<resource_id>.<8 hex of sha256(now)>.
func synthesizeAutomaticWebhookIdempotency(resourceID string) string {
	now := time.Now().UTC().UnixNano()
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%d", Provider, ResourceAutomaticWebhook, events.VerbEnsured, resourceID, now)))
	return fmt.Sprintf("%s.%s.%s.%s.%s", Provider, ResourceAutomaticWebhook, events.VerbEnsured, resourceID, hex.EncodeToString(sum[:4]))
}
