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

/**
 * The euphemized "Precisa de você" band. The readable signals today: (1) no
 * webhook subscription at all, and (2) a subscription with mTLS off. Those lead.
 */
export function AttentionBand({ webhooks }: AttentionBandProps) {
  if (webhooks.isLoading) {
    return <LoadingState label="Lendo a saúde do webhook…" />;
  }

  const noWebhook = webhooks.items.length === 0;
  const mtlsOff = webhooks.items.filter(isMtlsOff).slice(0, 6);

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
      </div>
    );
  }

  if (mtlsOff.length === 0) {
    return (
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
        <span>Nada precisa de você.</span>
      </p>
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
    </div>
  );
}
