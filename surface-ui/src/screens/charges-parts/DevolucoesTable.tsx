import type { DevolucaoItem } from "../../data";
import { formatMoney } from "../../data";
import { formatCreated, relativeCreated } from "../../data";
import { StatusDot, type DotTone } from "../shared/StatusDot";

export interface DevolucoesTableProps {
  devolucoes: DevolucaoItem[];
}

// One scoped stylesheet, matching the charges table: row hover lifts warm;
// container-query keeps the layout from collapsing on narrow hosts (scrolls).
//
// RULE #0 (HARDEST here): a devolução is money already moved — read-only
// history. Only the opaque refund id + valor/status/created are shown; never an
// endToEndId, a payer ref, or any chave. The adapter flattens devoluções from
// the pix legs WITHOUT those keys; this table never reintroduces them.
const TABLE_CSS = `
  .ef-dv-table { container-type: inline-size; width: 100%; }
  .ef-dv-scroll { overflow-x: auto; }
  .ef-dv-grid { width: 100%; min-width: 560px; border-collapse: separate; border-spacing: 0; }
  .ef-dv-grid th {
    text-align: left;
    font-family: var(--font-body);
    font-size: var(--fs-xs);
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--mut);
    padding: var(--sp-2) var(--sp-3);
    border-bottom: 1px solid var(--line);
    white-space: nowrap;
  }
  .ef-dv-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .ef-dv-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
    white-space: nowrap;
  }
  .ef-dv-mono { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
  .ef-dv-row { transition: background 100ms ease; }
  .ef-dv-row:hover { background: var(--sand); }
`;

/**
 * Map a BCB devolução status to a dot tone. EFI uses uppercase BCB statuses:
 * DEVOLVIDO = settled refund (ok), EM_PROCESSAMENTO = in flight (warn),
 * NAO_REALIZADO = not carried out (crit), anything else unknown (mut).
 */
export function devolucaoStatusTone(status: string): DotTone {
  const s = status.trim().toUpperCase();
  if (s === "DEVOLVIDO") return "ok";
  if (s === "EM_PROCESSAMENTO") return "warn";
  if (s === "NAO_REALIZADO") return "crit";
  return "mut";
}

/**
 * The devoluções roster inside the charge drill-down — money-already-moved
 * history, newest first. Columns: the devolução id (mono opaque ref), the valor
 * formatted to BRL, a status dot (DEVOLVIDO/EM_PROCESSAMENTO/NAO_REALIZADO,
 * read from the field), and the created timestamp. Refs only (rule #0).
 */
export function DevolucoesTable({ devolucoes }: DevolucoesTableProps) {
  const rows = [...devolucoes].sort((a, b) => {
    const ta = new Date(a.created).getTime() || 0;
    const tb = new Date(b.created).getTime() || 0;
    return tb - ta;
  });

  return (
    <div className="ef-dv-table">
      <style>{TABLE_CSS}</style>
      <div className="ef-dv-scroll">
        <table className="ef-dv-grid">
          <thead>
            <tr>
              <th>Devolução</th>
              <th style={{ textAlign: "right" }}>Valor</th>
              <th>Status</th>
              <th>Solicitada</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((d) => (
              <tr key={d.id} className="ef-dv-row">
                <td>
                  <span className="ef-dv-mono" style={{ color: "var(--ink)", fontWeight: 600 }} title={d.id}>
                    {d.id || "—"}
                  </span>
                </td>
                <td className="amount">{formatMoney(d.valor)}</td>
                <td>
                  <StatusDot tone={devolucaoStatusTone(d.status)} label={d.status || "—"} />
                </td>
                <td>
                  <span title={relativeCreated(d.created)} style={{ color: "var(--mut)" }}>
                    {formatCreated(d.created)}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
