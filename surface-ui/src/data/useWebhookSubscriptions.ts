import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, WebhookSubscriptionItem } from "./types";
import { mockEnabled, mockWebhookSubscriptions } from "./mock";

export interface WebhookSubscriptionsResult {
  items: WebhookSubscriptionItem[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits flat values; normalize every row into the strict shape the
// table relies on, dropping nothing and never throwing on a missing field.
function normalize(raw: Record<string, unknown>): WebhookSubscriptionItem {
  return {
    chave: (raw.chave ?? "").toString(),
    url: (raw.url ?? "").toString(),
    status: (raw.status ?? "").toString(),
    // `mtls` is the headline (Sec#2). Default to true only when truly absent —
    // a present `false` from the adapter (skip-mtls escape hatch) is preserved.
    mtls: raw.mtls === undefined ? true : raw.mtls === true
  };
}

/** True when mTLS is NOT enforced on this subscription — the one bad signal. */
export function isMtlsOff(s: WebhookSubscriptionItem): boolean {
  return s.mtls !== true;
}

/**
 * Every EFI Pix webhook subscription the instance configures — the
 * webhook-health pillar, the contract's canonical readable signal. The
 * mTLS-hardened webhook (Sec#2) is the headline; `mtls` is read straight from
 * the projection.
 */
export function useWebhookSubscriptions(instanceId: string | undefined): WebhookSubscriptionsResult {
  const mock = mockEnabled();
  // Under `?mock` pass an undefined handle so `useSurfaceQuery` stays disabled
  // (`enabled: !!instanceId`) — the hook is still called for stable order, but
  // it issues zero network and we return the fixture below.
  const query = useSurfaceQuery<ItemsEnvelope<WebhookSubscriptionItem>>(
    mock ? undefined : instanceId,
    "list-webhook-subscriptions",
    {}
  );

  if (mock) {
    return { items: mockWebhookSubscriptions(), isLoading: false, isError: false, error: null };
  }

  const raw = (query.data?.items ?? []) as unknown as Array<Record<string, unknown>>;
  return {
    items: raw.map(normalize),
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
