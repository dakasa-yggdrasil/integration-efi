import { useSurfaceQuery } from "@dakasa-yggdrasil/surface-toolkit";
import type { ChargeDetailObject, DevolucaoItem } from "./types";
import { mockEnabled, mockChargeDetail } from "./mock";

export interface ChargeDetailResult {
  detail: ChargeDetailObject | null;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
}

// One devolução row from the `charge-detail` object's `devolucoes` array.
// RULE #0: only the opaque refund id + valor/status/created are read — the
// adapter never projects the pix leg's endToEndId / chave or any payer field,
// and this normalizer never reads such a field.
function normalizeDevolucao(raw: Record<string, unknown>): DevolucaoItem {
  return {
    id: (raw.id ?? "").toString(),
    valor: (raw.valor ?? "").toString(),
    status: (raw.status ?? "").toString(),
    created: (raw.created ?? "").toString()
  };
}

// The adapter returns the `charge-detail` object (not a list envelope). Normalize
// the flat object + its devolucoes[] into the strict shape. RULE #0: only opaque
// refs are read; there is intentionally no devedor / pix-leg read here.
function normalize(raw: Record<string, unknown>): ChargeDetailObject {
  const rawDevolucoes = raw.devolucoes;
  const devolucoes = Array.isArray(rawDevolucoes)
    ? (rawDevolucoes as Array<Record<string, unknown>>).map(normalizeDevolucao)
    : [];
  return {
    txid: (raw.txid ?? "").toString(),
    valor: (raw.valor ?? "").toString(),
    status: (raw.status ?? "").toString(),
    tipo: (raw.tipo ?? "").toString(),
    created: (raw.created ?? "").toString(),
    expiracao: (raw.expiracao ?? "").toString(),
    devolucoes
  };
}

/**
 * The `charge-detail` drill-down read for a single Pix charge (the param
 * `txid`). The query stays disabled until both an instance handle and a
 * non-empty txid are present. Under `?mock` the network is bypassed and the
 * fixture detail is returned (scripted for the charge that has devoluções, a
 * synthesized concluída detail otherwise). RULE #0: opaque refs only — never
 * payer devedor/nome/cpf/email or the pix legs.
 */
export function useChargeDetail(
  instanceId: string | undefined,
  txid: string
): ChargeDetailResult {
  const mock = mockEnabled();
  const hasTxid = txid.trim() !== "";
  const query = useSurfaceQuery<Record<string, unknown>>(
    mock || !hasTxid ? undefined : instanceId,
    "charge-detail",
    { txid }
  );

  if (mock) {
    return {
      detail: hasTxid ? mockChargeDetail(txid) : null,
      isLoading: false,
      isError: false,
      error: null
    };
  }

  const data = query.data;
  return {
    detail: data && (data.txid ?? "").toString() !== "" ? normalize(data) : null,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error
  };
}
