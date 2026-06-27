// Shapes returned by the integration-efi adapter's surface queries
// (providers/efi/adapter/surface_query.go → onSurfaceQuery). The adapter emits
// flat JSON values; the published toolkit gives us no inference for
// `surface-query` responses, so every field is typed here and read defensively
// at the call site. Only fields the surface consumes are declared.
//
// IMPORTANT (keep in sync with surface_query.go's projections):
// the field NAMES below are the adapter's output keys.
//
// RULE #0 (HARDEST in the family — EFI is the contract's FORBIDDEN "pay your
// bill" example): an ops-internal Pix/finance view for the platform team, NEVER
// a customer Pix/billing view. Charge rows show ONLY opaque refs (txid, valor,
// status, tipo, created). The adapter's `projectCharge` DROPS `devedor` (payer
// nome / cpf / cnpj / email) and the pix legs (endToEndId) — there is
// intentionally NO payer field anywhere below, and the UI never renders a payer
// column.

/**
 * A row from `list-webhook-subscriptions` (one EFI Pix webhook subscription).
 *
 * The adapter projects observe_webhook_subscriptions →
 * `{chave, url, status, mtls}`. `status` is "active" for any present
 * subscription (EFI's webhook API has no per-subscription status field).
 * `mtls` reflects the Sec#2-hardened mTLS posture — `true` unless the
 * subscription was registered with the skip-mTLS escape hatch.
 */
export interface WebhookSubscriptionItem {
  /** The Pix key the subscription is registered against (operator-owned). */
  chave: string;
  /** The webhook URL EFI POSTs Pix callbacks to (operator-owned, not PII). */
  url: string;
  /** "active" for any present subscription. */
  status: string;
  /** Whether mTLS is enforced on delivery (the hardened default). */
  mtls: boolean;
}

/**
 * A row from `list-charges` (one recent Pix charge, for reconciliation context).
 *
 * The adapter projects observe_charges (cob / cobv) →
 * `{txid, valor, status, tipo, created}`.
 *
 * RULE #0: only opaque refs are projected — never the payer `devedor`
 * (nome / cpf / cnpj / email) EFI carries on the charge, never the pix legs
 * (endToEndId). The surface NEVER renders a payer column.
 */
export interface ChargeItem {
  /** The Pix transaction id (`txid`) — the opaque ref shown mono. */
  txid: string;
  /**
   * The charge amount as EFI returns it: a decimal REAIS string ("150.00",
   * "250.50") — NOT cents. Empty when absent. Formatted via formatMoney.
   */
  valor: string;
  /** BCB charge status (uppercase: "ATIVA" / "CONCLUIDA" / "REMOVIDA" / …). */
  status: string;
  /** Charge kind: "cob" (immediate) or "cobv" (due charge / boleto-Pix). */
  tipo: string;
  /** Creation time as an RFC3339 string ("2026-05-10T12:00:00Z"), or "". */
  created: string;
}

/** The envelope every list surface query returns: `{ items }`. */
export interface ItemsEnvelope<T> {
  items: T[];
}

/**
 * The deployment environment of the EFI instance. EFI homologation points at
 * the `pix-h` base URL (instance `sandbox: true`); production points at `pix`
 * (`sandbox: false`). Money-movement (refund / payout) would be REFUSED while
 * homolog, so the badge is mandatory and prominent.
 */
export type EfiEnvironment = "homolog" | "prod";
