import type { WebhookSubscriptionItem } from "../../data";
import { isMtlsOff, isMtlsUnknown } from "../../data";
import { StatusDot } from "../shared/StatusDot";

export interface WebhookTableProps {
  subscriptions: WebhookSubscriptionItem[];
}

// One scoped stylesheet: row hover lifts warm; container-query keeps the layout
// from collapsing on narrow hosts (the wrapper scrolls).
const TABLE_CSS = `
  .ef-wh-table { container-type: inline-size; width: 100%; }
  .ef-wh-scroll { overflow-x: auto; }
  .ef-wh-grid { width: 100%; min-width: 640px; border-collapse: separate; border-spacing: 0; }
  .ef-wh-grid th {
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
  .ef-wh-grid td {
    padding: var(--sp-3);
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
    font-size: var(--fs-sm);
    color: var(--body);
  }
  .ef-wh-row { transition: background 100ms ease; }
  .ef-wh-row:hover { background: var(--sand); }
  .ef-wh-row:hover .ef-wh-url { color: var(--honey); }
  .ef-wh-url { font-family: var(--font-mono, var(--font-body)); }
  .ef-wh-chave { font-family: var(--font-mono, var(--font-body)); color: var(--mut); }
`;

/**
 * The webhook subscriptions roster — the real data page, the contract's
 * canonical readable signal. Columns: the Pix `chave` (mono), the endpoint URL
 * (mono), a status dot (subscription present = active), and the mTLS dot
 * (ativo = ok / desligado = crit / não observado = warn). EFI's documented read
 * response does not report the registration-time mTLS flag. The adapter redacts
 * query values from the registered base URL before the surface sees it; there is
 * no payer column.
 */
export function WebhookTable({ subscriptions }: WebhookTableProps) {
  // mTLS-off subscriptions first, then unknown evidence, then confirmed mTLS.
  const rows = [...subscriptions].sort((a, b) => {
    const da = isMtlsOff(a) ? 0 : isMtlsUnknown(a) ? 1 : 2;
    const db = isMtlsOff(b) ? 0 : isMtlsUnknown(b) ? 1 : 2;
    if (da !== db) return da - db;
    return (a.url || a.chave).localeCompare(b.url || b.chave);
  });

  return (
    <div className="ef-wh-table">
      <style>{TABLE_CSS}</style>
      <div className="ef-wh-scroll">
        <table className="ef-wh-grid">
          <thead>
            <tr>
              <th>Chave Pix</th>
              <th>Endpoint</th>
              <th>Status</th>
              <th>mTLS</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => {
              const noMtls = isMtlsOff(s);
              const unknownMtls = isMtlsUnknown(s);
              return (
                <tr key={s.chave || s.url} className="ef-wh-row">
                  <td style={{ maxWidth: 220 }}>
                    <span
                      className="ef-wh-chave"
                      style={{
                        display: "block",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap"
                      }}
                      title={s.chave}
                    >
                      {s.chave || "—"}
                    </span>
                  </td>
                  <td style={{ maxWidth: 340 }}>
                    <span
                      className="ef-wh-url"
                      style={{
                        fontWeight: 600,
                        color: "var(--ink)",
                        transition: "color 100ms ease",
                        display: "block",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap"
                      }}
                      title={s.url}
                    >
                      {s.url || "—"}
                    </span>
                  </td>
                  <td>
                    <StatusDot
                      tone="ok"
                      label={s.status || "ativo"}
                      title="Assinatura de webhook presente"
                    />
                  </td>
                  <td>
                    <StatusDot
                      tone={noMtls ? "crit" : unknownMtls ? "warn" : "ok"}
                      label={noMtls ? "desligado" : unknownMtls ? "não observado" : "ativo"}
                      title={
                        noMtls
                          ? "Entrega sem mTLS (skip-mTLS)."
                          : unknownMtls
                            ? "A leitura da EFI não informa o modo mTLS; valide uma entrega autenticada."
                            : "mTLS informado como ativo na entrega."
                      }
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
