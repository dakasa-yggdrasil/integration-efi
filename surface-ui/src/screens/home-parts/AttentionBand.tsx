import type { CSSProperties } from "react";
import { Chip, Pill, LoadingState } from "@dakasa-yggdrasil/surface-toolkit";
import type { WebhookSubscriptionsResult } from "../../data";
import { isMtlsOff } from "../../data";

export interface AttentionBandProps {
  webhooks: WebhookSubscriptionsResult;
}

const ROW: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--sp-3)",
  padding: "var(--sp-3) var(--sp-4)",
  background: "var(--cream)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)",
  minWidth: 0
};

const NOTE: CSSProperties = {
  margin: 0,
  fontSize: "var(--fs-sm)",
  color: "var(--mut)",
  lineHeight: 1.55
};

/**
 * The euphemized "Precisa de você" band. The tone is supportive, never "ALERTA".
 *
 * HONEST by construction: reconciliation drift and payout alerts are NOT
 * readable yet (drift needs a cross-system join EFI charges vs
 * identities.webhook_event_efi via core; payout history has no observe op and
 * the prólabore decision lives in the cash-loop workflow), so we never lead with
 * a fabricated "you have N drifts". The real, readable signals today are:
 * (1) NO webhook subscription at all (EFI deliveries would have nowhere to land)
 * and (2) a subscription with mTLS OFF (the Sec#2 hardening defeated). Those
 * lead. When nothing is readable-critical, we say so plainly and note what lands
 * once the cross-system join / cash-loop read is wired.
 */
export function AttentionBand({ webhooks }: AttentionBandProps) {
  if (webhooks.isLoading) {
    return <LoadingState label="Lendo a saúde do webhook…" />;
  }

  const noWebhook = webhooks.items.length === 0;
  const mtlsOff = webhooks.items.filter(isMtlsOff).slice(0, 6);

  const futureNote = (
    <p style={NOTE}>
      O <strong>drift de reconciliação</strong> (charges da EFI vs{" "}
      <code>identities.webhook_event_efi</code>) e os <strong>alertas de payout / prólabore</strong> entram aqui quando
      o join cross-system via core e a leitura do <strong>cash-loop</strong> forem ligados — por ora não são legíveis e
      não inventamos um número.
    </p>
  );

  if (noWebhook) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
        <div style={ROW}>
          <div style={{ display: "flex", flexDirection: "column", gap: "2px", minWidth: 0, flex: 1 }}>
            <span style={{ fontSize: "var(--fs-sm)", fontWeight: 500, color: "var(--ink)" }}>
              Nenhuma assinatura de webhook configurada
            </span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)" }}>
              <Chip label="webhook" tone="team" />
              <Pill label="ausente" tone="crit" preserveCase />
            </span>
          </div>
        </div>
        {futureNote}
      </div>
    );
  }

  if (mtlsOff.length === 0) {
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
        <p
          style={{
            margin: 0,
            display: "flex",
            alignItems: "center",
            gap: "var(--sp-2)",
            fontSize: "var(--fs-md)",
            color: "var(--mut)",
            lineHeight: 1.5
          }}
        >
          <span aria-hidden="true" style={{ color: "var(--ok)", fontWeight: 700 }}>
            ✓
          </span>
          <span>Nada crítico legível agora. O webhook está ativo com mTLS.</span>
        </p>
        {futureNote}
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
        {mtlsOff.map((s) => (
          <div key={s.chave || s.url} style={ROW}>
            <div style={{ display: "flex", flexDirection: "column", gap: "2px", minWidth: 0, flex: 1 }}>
              <span
                style={{
                  fontFamily: "var(--font-mono, var(--font-body))",
                  fontSize: "var(--fs-sm)",
                  fontWeight: 500,
                  color: "var(--ink)",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  whiteSpace: "nowrap"
                }}
                title={s.url}
              >
                {s.url || s.chave}
              </span>
              <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--sp-2)" }}>
                <Chip label="webhook" tone="team" />
                <Pill label="mTLS desligado" tone="crit" preserveCase />
              </span>
            </div>
          </div>
        ))}
      </div>
      {futureNote}
    </div>
  );
}
