import type { CSSProperties } from "react";
import {
  TierTwoShell,
  KpiTile,
  LoadingState,
  useCollaboratorScope,
  useDefaultInstance
} from "@dakasa-yggdrasil/surface-toolkit";
import {
  useEnvironment,
  mockEnabled,
  mockCollaboratorScope,
  MOCK_INSTANCE_ID
} from "../data";
import { GatedAction } from "./shared/GatedAction";
import { EnvironmentBadge } from "./shared/EnvironmentBadge";
import { kpiSubtext } from "./shared/kpiQualifier";

const SHELL_WRAP: CSSProperties = {
  width: "100%",
  maxWidth: 1120,
  margin: "0 auto",
  padding: "var(--sp-6) var(--sp-5) var(--sp-7)"
};

const KPI_GRID = `
  .ef-rf-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .ef-rf-kpis { grid-template-columns: 1fr; } }
`;

const SECTION_TITLE: CSSProperties = {
  margin: 0,
  fontFamily: "var(--font-heading)",
  fontSize: "var(--fs-lg)",
  fontWeight: 500,
  color: "var(--ink)"
};

const CARD: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--sp-2)",
  padding: "var(--sp-4) var(--sp-5)",
  background: "var(--cream)",
  border: "1px solid var(--line)",
  borderRadius: "var(--r-md)"
};

// What this page WILL show once a refund read exists — framed honestly as
// needs-work, NEVER fabricated. The refund (devolução) write path is admin and
// out of v1, and the refund HISTORY has no observe op, so we never list one.
const NEEDS_WORK: Array<{ label: string; detail: string }> = [
  {
    label: "Histórico de devoluções (refunds)",
    detail:
      "As devoluções Pix já efetuadas (PUT /v2/pix/{e2eid}/devolucao/{id}). O adapter expõe refund_charge como write (money-movement), mas não há op observe para o histórico de devoluções — por isso não mostramos uma lista, seria inventada."
  },
  {
    label: "Devolução como remediação de ops",
    detail:
      "Estornar uma charge é uma remediação de ops (corrigir uma cobrança errada), não um fluxo de produto. Quando o caminho de escrita for ligado, a devolução entra aqui com idempotência + auditoria, restrita ao admin, e recusada enquanto homolog."
  },
  {
    label: "Chargebacks",
    detail:
      "O tratamento de chargeback (handle_chargeback) é reconstruído a jusante a partir do log de eventos; um read dedicado para o estado de contestação é needs-work."
  }
];

export function Refunds() {
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const scope = mock ? mockCollaboratorScope() : liveScope;
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const env = useEnvironment();

  // No refund data is read at all (needs-work); the instance handle only keeps
  // the chrome consistent.
  void instanceId;

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="ef-rf-kpis">
        <KpiTile eyebrow="Devoluções recentes" value="—" chart={kpiSubtext("sem op de leitura", false)} />
        <KpiTile eyebrow="Chargebacks" value="—" chart={kpiSubtext("reconstruído via evento", false)} />
        <KpiTile eyebrow="Estado" value="—" chart={kpiSubtext("admin · remediação", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading) {
      return <LoadingState label="Preparando a página de refunds…" />;
    }
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* the honest framing */}
        <section
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--sp-3)",
            padding: "var(--sp-5) var(--sp-6)",
            background: "var(--sand2)",
            border: "1px solid var(--line)",
            borderRadius: "var(--r-lg)"
          }}
        >
          <p style={{ margin: 0, fontSize: "var(--fs-md)", color: "var(--body)", lineHeight: 1.55 }}>
            A <strong>devolução (refund)</strong> é uma <strong>remediação de ops</strong> — corrigir uma cobrança
            errada — não um botão transacional de produto. O <strong>histórico de devoluções</strong> não é lido por
            esta surface (sem op de leitura — <em>needs-work</em>); não inventamos uma lista.
          </p>
        </section>

        {/* what's needs-work */}
        <section style={{ display: "flex", flexDirection: "column", gap: "var(--sp-3)" }}>
          <h3 style={SECTION_TITLE}>O que falta conectar</h3>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-2)" }}>
            {NEEDS_WORK.map((v) => (
              <div key={v.label} style={CARD}>
                <span style={{ fontWeight: 600, color: "var(--ink)" }}>{v.label}</span>
                <span style={{ fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>{v.detail}</span>
              </div>
            ))}
          </div>
        </section>

        {/* money-movement: refund_charge — admin-tier, gated + disabled, refused in homolog */}
        <GatedAction
          need="efi.refunds.create"
          perms={scope.perms}
          env={env}
          eyebrow="Remediação"
          label="Estornar uma charge (devolução Pix) é movimentação de dinheiro — admin, fora da v1. Quando o caminho de escrita for ligado, a devolução entra aqui como remediação de ops (com idempotência + auditoria), nunca um botão transacional solto."
          hint="refund_charge é admin, fora da v1, e recusado enquanto homolog."
        />
      </div>
    );
  }

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Refunds"
        subtitle="Devolução (refund) como remediação de ops — admin, fora da v1, recusada em homolog. O histórico de devoluções é needs-work (sem op de leitura)."
        teamChips={<EnvironmentBadge env={env} size="sm" />}
        kpis={instanceLoading ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
