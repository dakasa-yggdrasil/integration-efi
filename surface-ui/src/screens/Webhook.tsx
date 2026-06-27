import type { CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  EmptyState,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useWebhookSubscriptions,
  useEfiBase,
  useEnvironment,
  webhookHref,
  isMtlsOff,
  mockEnabled,
  MOCK_INSTANCE_ID
} from "../data";
import { WebhookTable } from "./webhook-parts";
import { EnvironmentBadge } from "./shared/EnvironmentBadge";
import { kpiDelta, kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .ef-wh-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .ef-wh-kpis { grid-template-columns: 1fr; } }
`;

const NOTE: CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  gap: "var(--sp-3)",
  padding: "var(--sp-3) var(--sp-4)",
  background: "var(--sand2)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)"
};

export function Webhook() {
  const mock = mockEnabled();
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const efiBase = useEfiBase();
  const env = useEnvironment();

  const webhooks = useWebhookSubscriptions(instanceId);

  const total = webhooks.items.length;
  const mtlsOff = webhooks.items.filter(isMtlsOff).length;
  const mtlsOn = total - mtlsOff;

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="ef-wh-kpis">
        <KpiTile eyebrow="Assinaturas" value={total} chart={kpiSubtext(total === 0 ? "nenhuma" : "registrada(s)", false)} />
        <KpiTile
          eyebrow="Com mTLS"
          value={mtlsOn}
          delta={kpiDelta(`${mtlsOff} sem mTLS`, mtlsOff > 0)}
          chart={kpiSubtext("endurecido (Sec#2)", mtlsOff > 0)}
        />
        <KpiTile
          eyebrow="Sem mTLS"
          value={mtlsOff}
          delta={kpiDelta("escotilha skip-mTLS", mtlsOff > 0)}
          chart={kpiSubtext("nenhuma", mtlsOff > 0)}
        />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading || webhooks.isLoading) {
      return <LoadingState label="Lendo as assinaturas de webhook…" />;
    }
    if (webhooks.isError) {
      return (
        <EmptyState
          title="Não consegui ler o webhook"
          description={
            webhooks.error instanceof Error ? webhooks.error.message : "Tente novamente em instantes."
          }
        />
      );
    }
    if (total === 0) {
      return (
        <EmptyState
          title="Nenhuma assinatura de webhook"
          description="Esta conta EFI ainda não tem uma assinatura de webhook Pix registrada para este token. Sem ela, as entregas Pix não têm onde aterrissar."
        />
      );
    }
    const dashHref = webhookHref(efiBase);
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* verify-signature + freshness diagnostic note */}
        <div style={NOTE}>
          <span aria-hidden="true" style={{ color: "var(--mut)", fontWeight: 700, marginTop: "1px" }}>
            ◦
          </span>
          <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
            O webhook EFI é entregue por <strong>mTLS</strong> (o endpoint endurecido no Sec#2); a{" "}
            <strong>verificação de assinatura</strong> é feita pelo adapter no recebimento (
            <code>verify_webhook_signature</code> / handshake mTLS). A <strong>frescura da última entrega</strong>{" "}
            (quando o último callback Pix chegou) não é lida por esta surface hoje (<em>needs-work</em>) — para o
            histórico de entregas, abra o webhook na EFI (<strong>↗</strong>).
          </span>
        </div>

        <WebhookTable subscriptions={webhooks.items} />

        {/* deep-link to native webhook settings */}
        <div>
          {dashHref ? (
            <a
              href={dashHref}
              target="_blank"
              rel="noreferrer"
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: "var(--sp-2)",
                fontSize: "var(--fs-sm)",
                fontWeight: 700,
                color: "var(--honey)",
                textDecoration: "none"
              }}
            >
              Webhook na EFI <span aria-hidden="true">↗</span>
            </a>
          ) : (
            <span
              title="Link para a EFI nativa indisponível: o host do console ainda não é exposto por um surface read."
              style={{ fontSize: "var(--fs-sm)", fontWeight: 700, color: "var(--mut)", opacity: 0.7 }}
            >
              Webhook na EFI <span aria-hidden="true">↗</span>
            </span>
          )}
        </div>
      </div>
    );
  }

  const chromeBusy = instanceLoading || webhooks.isLoading || webhooks.isError;

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Webhook & mTLS"
        subtitle="A assinatura de webhook Pix da EFI — chave, URL e o dot de mTLS (o webhook endurecido no Sec#2). A frescura da última entrega é needs-work."
        teamChips={<EnvironmentBadge env={env} size="sm" />}
        kpis={chromeBusy ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
