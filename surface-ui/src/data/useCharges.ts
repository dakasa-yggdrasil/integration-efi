import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ItemsEnvelope, ChargeItem } from "./types";
import { mockEnabled, mockCharges } from "./mock";

export interface ChargesResult {
  items: ChargeItem[];
  /** The {inicio, fim} window the roster covers (for an honest "na janela"). */
  windowDays: number;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// The adapter emits flat values; normalize every row into the strict shape.
// RULE #0: only opaque refs (txid, valor, status, tipo, created) ever appear —
// the adapter never projects payer devedor/nome/cpf/email or the pix legs, and
// this normalizer never reads such a field.
function normalize(raw: Record<string, unknown>): ChargeItem {
  return {
    txid: (raw.txid ?? "").toString(),
    valor: (raw.valor ?? "").toString(),
    status: (raw.status ?? "").toString(),
    tipo: (raw.tipo ?? "").toString(),
    created: (raw.created ?? "").toString()
  };
}

/** An RFC3339 timestamp `days` ago / now, for the statement window params. */
function isoDaysAgo(days: number): string {
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}
function isoNow(): string {
  return new Date().toISOString();
}

/**
 * Recent Pix charges for reconciliation context (config-grade opaque refs only
 * — never payer data). The adapter's `list-charges` requires a window
 * `{inicio, fim}` (or a single `{txid}`); we read a trailing `windowDays`
 * window. A status string narrows the BCB charge status when supplied.
 */
export function useCharges(
  instanceId: string | undefined,
  opts: { windowDays?: number; status?: string } = {}
): ChargesResult {
  const windowDays = opts.windowDays ?? 30;
  const mock = mockEnabled();

  const params: Record<string, unknown> = {
    inicio: isoDaysAgo(windowDays),
    fim: isoNow()
  };
  if (opts.status && opts.status.trim() !== "") {
    params.status = opts.status.trim();
  }

  const query = useSurfaceQuery<ItemsEnvelope<ChargeItem>>(
    mock ? undefined : instanceId,
    "list-charges",
    params
  );

  if (mock) {
    return { items: mockCharges(), windowDays, isLoading: false, isError: false, error: null };
  }

  const raw = (query.data?.items ?? []) as unknown as Array<Record<string, unknown>>;
  return {
    items: raw.map(normalize),
    windowDays,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
