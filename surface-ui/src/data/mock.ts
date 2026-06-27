// DEV-only fixtures so Home + the four detail pages can be designed and
// verified populated, without a live EFI instance. Gated by `mockEnabled()`
// (import.meta.env.DEV + a `?mock` URL param) at every call site, so this is
// dead code tree-shaken out of any production build. The data is realistic and
// internally consistent: environment = HOMOLOG, exactly 1 webhook subscription
// (mTLS enforced — the Sec#2 hardened webhook), and ~8 recent Pix charges (a mix
// of cob / cobv, statuses ATIVA / CONCLUIDA / REMOVIDA, varied valores).
//
// There is intentionally NO reconciliation-drift fixture and NO payout/prólabore
// fixture — the adapter has no read op for either (reconciliation drift needs a
// cross-system join via core; payout history has no observe op, and the
// prólabore decision lives in the cash-loop workflow). The Home never fabricates
// a drift count or a payout, and the Payouts / Refunds pages are honestly
// needs-work.
//
// RULE #0: every charge row carries ONLY opaque refs (txid, valor, status,
// tipo, created). There is NO payer (devedor / nome / cpf / email / pix key)
// anywhere — the adapter drops it and this fixture never reintroduces it.

import type { CollaboratorScope } from "@dakasa-yggdrasil/surface-toolkit";
import type { WebhookSubscriptionItem, ChargeItem, EfiEnvironment } from "./types";

/**
 * DEV `?mock` switch, shared across the hooks so every read short-circuits the
 * network together. Never true in a production build (guarded on DEV).
 */
export function mockEnabled(): boolean {
  return (
    import.meta.env.DEV &&
    typeof location !== "undefined" &&
    new URLSearchParams(location.search).has("mock")
  );
}

/** Fake instance id used to satisfy the surface-query handle under `?mock`. */
export const MOCK_INSTANCE_ID = "mock-efi-instance";

/** The EFI account label shown in the headline under `?mock`. */
export const MOCK_INSTANCE_LABEL = "DaKasa · EFI Pix";

/**
 * The mock environment. Per the family spec, EFI defaults to HOMOLOG in the
 * mock so the mandatory environment badge is exercised in its safe state, and
 * the (gated, disabled) money-movement affordances correctly state they would
 * be REFUSED while homolog.
 */
export const MOCK_ENVIRONMENT: EfiEnvironment = "homolog";

/**
 * Fixture EFI console host for `?mock` deep-links. In production this host is
 * not yet wired through a surface read, so the live deep-links degrade honestly
 * (disabled "↗"); under the mock we point at the real console host so the "↗"
 * affordance can be reviewed as a working link.
 */
export const MOCK_EFI_BASE = "https://sejaefi.com.br/conta";

/**
 * Fully-offline collaborator + permission context for `?mock` review. Under the
 * mock gate this replaces the network-backed `useCollaboratorScope()` so the
 * surface renders standalone with zero requests to /me, provisioning-status, or
 * the manifests list. Tier is `admin` and the perms cover every EFI
 * money-movement capability the surface gates on, so the (gated, disabled)
 * "Em breve" affordances are visible for review. Never reached in production
 * (gated on {@link mockEnabled}).
 */
export function mockCollaboratorScope(): CollaboratorScope {
  return {
    collaborator: {
      id: "giomaster",
      slug: "Giomaster",
      display_name: "Giovanni Rios Martins",
      primary_email: "giovanni.martins@dakasa.me",
      status: "active"
    },
    teams: [{ teamId: "mock-team", slug: "plataforma", githubSlug: "plataforma" }],
    tier: "admin",
    perms: [
      "manage-integrations",
      "efi.refunds.create",
      "efi.payouts.create",
      "efi.webhooks.ensure"
    ],
    isLoading: false,
    isError: false
  };
}

// ---------------------------------------------------------------- webhook

// Exactly ONE webhook subscription — the mTLS-hardened endpoint (Sec#2). EFI
// registers a single webhook per Pix key; mTLS is enforced (the headline this
// surface exists to highlight). This is the one strong, real "esteira saudável"
// signal the adapter can read today.
//
// [chave, url, status, mtls]
const WEBHOOK_ROWS: Array<[string, string, string, boolean]> = [
  ["pix@dakasa.me", "https://webhook-h.dakasa.me/efi/webhook/pix", "active", true]
];

export function mockWebhookSubscriptions(): WebhookSubscriptionItem[] {
  return WEBHOOK_ROWS.map(([chave, url, status, mtls]) => ({ chave, url, status, mtls }));
}

// ---------------------------------------------------------------- charges

// ~8 recent Pix charges for the reconciliation roster. A mix of cob (immediate)
// and cobv (due charge), statuses ATIVA / CONCLUIDA / REMOVIDA, varied valores.
// `valor` is a decimal REAIS string exactly as EFI returns it (NOT cents).
// `created` is an RFC3339 string. NO payer data anywhere (rule #0).
//
// [txidSuffix, valor, status, tipo, minutesAgo]
const CHARGE_ROWS: Array<[string, string, string, string, number]> = [
  ["a1concluida0001", "150.00", "CONCLUIDA", "cob", 6],
  ["a2ativa00000002", "89.90", "ATIVA", "cob", 24],
  ["a3concluida0003", "1250.00", "CONCLUIDA", "cobv", 58],
  ["a4ativa00000004", "42.00", "ATIVA", "cob", 96],
  ["a5removida00005", "320.50", "REMOVIDA", "cob", 140],
  ["a6concluida0006", "2480.00", "CONCLUIDA", "cobv", 205],
  ["a7ativa00000007", "75.00", "ATIVA", "cob", 268],
  ["a8concluida0008", "499.99", "CONCLUIDA", "cob", 332]
];

export function mockCharges(): ChargeItem[] {
  const now = Date.now();
  return CHARGE_ROWS.map(([txidSuffix, valor, status, tipo, minutesAgo]) => ({
    txid: "efi-tx-" + txidSuffix,
    valor,
    status,
    tipo,
    created: new Date(now - minutesAgo * 60_000).toISOString()
  }));
}
