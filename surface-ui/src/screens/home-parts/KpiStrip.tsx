import type { CSSProperties } from "react";
import { KpiTile } from "@dakasa-yggdrasil/surface-toolkit";
import type { EfiPulse } from "../../data";
import { kpiDelta, kpiSubtext } from "../shared/kpiQualifier";

export interface KpiStripProps {
  pulse: EfiPulse;
}

// Dense, responsive grid of KpiTiles. Reflows by the host width (container
// query), never the viewport — four terse finance-OPS facts that read the same
// on a wide console or a narrow panel.
const GRID = `
  .ef-kpi-strip {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
  @container (max-width: 1040px) {
    .ef-kpi-strip { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  }
  @container (max-width: 480px) {
    .ef-kpi-strip { grid-template-columns: 1fr; }
  }
`;

const WRAP: CSSProperties = { containerType: "inline-size", width: "100%" };

/**
 * KPI polish: the directional arrow misleads on a static fact, so neutral/good
 * facts carry a plain muted subtext (no arrow) via the `chart` slot, and only a
 * genuinely bad signal (an absent webhook, or mTLS off) gets a `delta` with the
 * crit ↓.
 *
 * Honest handling of the two un-readable facts — NEVER a fabricated number:
 *  - Reconciliação shows "— needs-work" (the drift EFI charges vs
 *    identities.webhook_event_efi needs a cross-system join via core).
 *  - Payouts shows "— needs-work" (no observe op; the prólabore decision lives
 *    in the cash-loop workflow).
 */
export function KpiStrip({ pulse }: KpiStripProps) {
  const noWebhook = !pulse.hasWebhook;
  const mtlsBad = pulse.webhooksMtlsOff > 0;
  const webhookBad = noWebhook || mtlsBad;

  return (
    <div style={WRAP}>
      <style>{GRID}</style>
      <div className="ef-kpi-strip">
        <KpiTile
          eyebrow="Webhook mTLS"
          value={noWebhook ? "—" : pulse.webhooksMtls > 0 ? "ativo" : "inativo"}
          delta={kpiDelta(noWebhook ? "sem webhook" : "mTLS desligado", webhookBad)}
          chart={kpiSubtext(
            noWebhook ? "nenhuma assinatura" : `${pulse.webhooksMtls} de ${pulse.webhooks} com mTLS`,
            webhookBad
          )}
        />
        <KpiTile
          eyebrow={`Charges (${pulse.chargeWindowDays}d)`}
          value={pulse.charges}
          chart={kpiSubtext("refs, sem dados de pagador", false)}
        />
        <KpiTile eyebrow="Reconciliação" value="—" chart={kpiSubtext("needs-work", false)} />
        <KpiTile eyebrow="Payouts" value="—" chart={kpiSubtext("needs-work", false)} />
      </div>
    </div>
  );
}
