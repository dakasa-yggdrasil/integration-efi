import { useMemo } from "react";
import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, WebhookSubscriptionItem } from "./types";
import { mockEnabled, mockWebhookSubscriptions } from "./mock";

export interface WebhookSubscriptionsResult {
  items: WebhookSubscriptionItem[];
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// Pix did not exist before 2020. Use an explicit, stable lower bound so the
// provider-required list window covers every possible DaKasa subscription
// without silently dropping an older registration. The upper bound remains
// the time of the read so newly-created subscriptions are visible.
const WEBHOOK_HISTORY_START = "2020-01-01T00:00:00.000Z";

// The adapter emits flat values; normalize every row into the strict shape the
// table relies on, dropping nothing and never throwing on a missing field.
function normalize(raw: Record<string, unknown>): WebhookSubscriptionItem {
  return {
    chave: (raw.chave ?? "").toString(),
    url: (raw.url ?? "").toString(),
    status: (raw.status ?? "").toString(),
    // Preserve all three honest states. EFI's documented observer response
    // omits this field, which is unknown rather than healthy by default.
    mtls: raw.mtls === true ? true : raw.mtls === false ? false : null
  };
}

/** True when mTLS is NOT enforced on this subscription — the one bad signal. */
export function isMtlsOff(s: WebhookSubscriptionItem): boolean {
  return s.mtls === false;
}

/** True when EFI's read API did not prove the registration-time mTLS mode. */
export function isMtlsUnknown(s: WebhookSubscriptionItem): boolean {
  return s.mtls === null;
}

/**
 * Every EFI Pix webhook subscription the instance configures — the
 * webhook-health pillar, the contract's canonical readable signal. The
 * mTLS-hardened webhook (Sec#2) is the headline; `mtls` is read straight from
 * the projection.
 */
export function useWebhookSubscriptions(instanceId: string | undefined): WebhookSubscriptionsResult {
  const mock = mockEnabled();
  const params = useMemo<Record<string, unknown>>(
    () => ({ inicio: WEBHOOK_HISTORY_START, fim: new Date().toISOString() }),
    [instanceId]
  );
  // Under `?mock` pass an undefined handle so `useSurfaceQuery` stays disabled
  // (`enabled: !!instanceId`) — the hook is still called for stable order, but
  // it issues zero network and we return the fixture below.
  const query = useSurfaceQuery<ItemsEnvelope<WebhookSubscriptionItem>>(
    mock ? undefined : instanceId,
    "list-webhook-subscriptions",
    params
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
