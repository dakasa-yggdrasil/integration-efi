import { Chip } from "@dakasa-yggdrasil/surface-toolkit";
import type { ChargeItem } from "../../data";
import { formatMoney } from "../../data";
import { StatusDot, chargeStatusTone } from "../shared/StatusDot";
import { formatCreated, relativeCreated } from "../../data";

export interface ChargeTableProps {
  charges: ChargeItem[];
}

// One scoped stylesheet: row hover lifts warm; container-query keeps the layout
// from collapsing on narrow hosts (the wrapper scrolls).
//
// RULE #0 (HARDEST here — EFI is the contract's FORBIDDEN "pay your bill"
// example): there is NO payer column. The only data shown are the opaque refs
// (txid, valor, status, tipo, created) — never a payer nome/cpf/email/pix key.
// The adapter's projectCharge drops `devedor`; this table never reintroduces it.
const TABLE_CSS = `
  .ef-ch-table { container-type: inline-size; width: 100%; }
  .ef-ch-scroll { overflow-x: auto; }
  .ef-ch-grid { width: 100%; min-width: 640px; border-collapse: separate; border-spacing: 0; }
  .ef-ch-grid th {
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
  .ef-ch-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .ef-ch-grid td.amount {
    text-align: right;
    font-family: var(--font-mono, var(--font-body));
    font-weight: 600;
    color: var(--ink);
    white-space: nowrap;
  }
  .ef-ch-mono { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
  .ef-ch-row { transition: background 100ms ease; }
  .ef-ch-row:hover { background: var(--sand); }
  .ef-ch-row:hover .ef-ch-txid { color: var(--honey); }
`;

/** The cob / cobv kind chip (immediate vs due charge). */
function TipoChip({ tipo }: { tipo: string }) {
  const t = tipo.trim().toLowerCase();
  if (t === "cobv") return <Chip label="cobv" tone="team" preserveCase />;
  if (t === "cob") return <Chip label="cob" tone="neutral" preserveCase />;
  return <Chip label={tipo || "—"} tone="neutral" preserveCase />;
}

/**
 * The recent-charges roster — reconciliation context, refs only. Columns: the
 * txid (mono opaque ref), the valor formatted to BRL (from EFI's decimal-reais
 * string), a status dot (CONCLUIDA/ATIVA/REMOVIDA, read from the field), the
 * tipo chip (cob/cobv), and the created timestamp. There is intentionally NO
 * payer column and NO "↗" per-row (EFI has no per-charge native deep-link the
 * adapter exposes; the page-level "↗" goes to the EFI Pix area).
 */
export function ChargeTable({ charges }: ChargeTableProps) {
  const rows = [...charges].sort((a, b) => {
    const ta = new Date(a.created).getTime() || 0;
    const tb = new Date(b.created).getTime() || 0;
    return tb - ta;
  });

  return (
    <div className="ef-ch-table">
      <style>{TABLE_CSS}</style>
      <div className="ef-ch-scroll">
        <table className="ef-ch-grid">
          <thead>
            <tr>
              <th>Txid</th>
              <th style={{ textAlign: "right" }}>Valor</th>
              <th>Status</th>
              <th>Tipo</th>
              <th>Criada</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((c) => (
              <tr key={c.txid} className="ef-ch-row">
                <td style={{ maxWidth: 280 }}>
                  <span
                    className="ef-ch-txid ef-ch-mono"
                    style={{
                      fontWeight: 600,
                      color: "var(--ink)",
                      transition: "color 100ms ease",
                      display: "block",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap"
                    }}
                    title={c.txid}
                  >
                    {c.txid || "—"}
                  </span>
                </td>
                <td className="amount">{formatMoney(c.valor)}</td>
                <td>
                  <StatusDot tone={chargeStatusTone(c.status)} label={c.status || "—"} />
                </td>
                <td>
                  <TipoChip tipo={c.tipo} />
                </td>
                <td>
                  <span title={relativeCreated(c.created)} style={{ color: "var(--mut)" }}>
                    {formatCreated(c.created)}
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
