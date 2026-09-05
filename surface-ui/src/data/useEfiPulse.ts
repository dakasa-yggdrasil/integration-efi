import { useWebhookSubscriptions, isMtlsOff, isMtlsUnknown } from "./useWebhookSubscriptions";
import { useCharges } from "./useCharges";

export interface EfiPulse {
  /** Total configured webhook subscriptions. */
  webhooks: number;
  /** Subscriptions with mTLS explicitly reported as enforced. */
  webhooksMtls: number;
  /** Subscriptions WITHOUT mTLS — the readable "precisa de você" signal. */
  webhooksMtlsOff: number;
  /** Subscriptions whose mTLS registration mode is not returned by EFI. */
  webhooksMtlsUnknown: number;
  /** Whether at least one webhook subscription exists. */
  hasWebhook: boolean;
  /** Recent charges in the window (reconciliation roster size). */
  charges: number;
  /** The window the charge count covers, in days. */
  chargeWindowDays: number;
  isLoading: boolean;
  isError: boolean;
}

/**
 * One derived read of the EFI account's OPS posture, composed from the two
 * wired reads (webhook subscriptions + recent charges). Powers the technical
 * Home headline + KPI strip. Every value is a bare, real fact an operator can
 * act on.
 *
 * Deliberately NO `reconciliationDrift` and NO `payouts` here — the adapter has
 * no read op for either: reconciliation drift (EFI charges vs
 * identities.webhook_event_efi) needs a cross-system join via core, and payout /
 * prólabore history has no observe op (the prólabore decision lives in the
 * cash-loop workflow). Fabricating either would be a lie; the Home shows them
 * honestly as "— needs-work".
 */
export function useEfiPulse(instanceId: string | undefined): EfiPulse {
  const webhooks = useWebhookSubscriptions(instanceId);
  const charges = useCharges(instanceId, { windowDays: 30 });

  const mtlsOff = webhooks.items.filter(isMtlsOff).length;
  const mtlsUnknown = webhooks.items.filter(isMtlsUnknown).length;

  return {
    webhooks: webhooks.items.length,
    webhooksMtls: webhooks.items.length - mtlsOff - mtlsUnknown,
    webhooksMtlsOff: mtlsOff,
    webhooksMtlsUnknown: mtlsUnknown,
    hasWebhook: webhooks.items.length > 0,
    charges: charges.items.length,
    chargeWindowDays: charges.windowDays,
    isLoading: webhooks.isLoading || charges.isLoading,
    isError: webhooks.isError || charges.isError
  };
}
