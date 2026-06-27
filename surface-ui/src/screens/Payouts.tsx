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
  .ef-po-kpis {
    display: grid;
    gap: var(--sp-3);
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  @container (max-width: 560px) { .ef-po-kpis { grid-template-columns: 1fr; } }
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

// What this page WILL show once a payout read exists — framed honestly as
// needs-work, NEVER fabricated as present data. There is no observe_payouts op,
// and the prólabore decision lives in the cash-loop workflow, so faking a payout
// list here would be the worst kind of lie on the money-movement rail.
const NEEDS_WORK: Array<{ label: string; detail: string }> = [
  {
    label: "Histórico de payouts / repasses",
    detail:
      "Os Pix de saída (PUT /v3/gn/pix/{idEnvio}) já confirmados e agendados. O adapter não tem uma op observe_payouts hoje — create_payout é write-only (IntermediateIrreversible) e a EFI não expõe um list-envios que o adapter embrulhe. Por isso não mostramos uma lista — seria inventada."
  },
  {
    label: "Prólabore (competência & valor)",
    detail:
      "A competência e o valor do pró-labore de cada sócio NÃO são decididos aqui. O seam é claro: o employment-clt decide competência/valor e o cash-loop workflow decide quem/quanto; a EFI apenas executa o Pix. Esta surface é confirm + observe — nunca a fonte da decisão."
  },
  {
    label: "Confirmar + observar (não decidir)",
    detail:
      "Quando o caminho de escrita for ligado, esta página confirma um payout que o cash-loop já decidiu (com idempotência + auditoria) e observa o resultado — recusado enquanto homolog. Ela nunca origina o valor nem o destino."
  }
];

export function Payouts() {
  const mock = mockEnabled();
  const liveScope = useCollaboratorScope();
  const scope = mock ? mockCollaboratorScope() : liveScope;
  const { data: liveInstanceId, isLoading: liveInstanceLoading } = useDefaultInstance("efi");
  const instanceId = mock ? MOCK_INSTANCE_ID : liveInstanceId;
  const instanceLoading = mock ? false : liveInstanceLoading;
  const env = useEnvironment();

  // No payout data is read at all (needs-work), so the instance handle is only
  // used to keep the chrome consistent; we never query payout state.
  void instanceId;

  const kpis = (
    <div style={{ containerType: "inline-size", width: "100%" }}>
      <style>{KPI_GRID}</style>
      <div className="ef-po-kpis">
        <KpiTile eyebrow="Payouts recentes" value="—" chart={kpiSubtext("sem op de leitura", false)} />
        <KpiTile eyebrow="Próximo prólabore" value="—" chart={kpiSubtext("decide o cash-loop", false)} />
        <KpiTile eyebrow="Estado" value="—" chart={kpiSubtext("confirm + observe", false)} />
      </div>
    </div>
  );

  function body() {
    if (instanceLoading) {
      return <LoadingState label="Preparando a página de payouts…" />;
    }
    return (
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--sp-6)" }}>
        {/* the honest framing — surface NEVER decides who/how-much */}
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
            Esta surface <strong>nunca decide quem ou quanto pagar</strong>. O valor e o destino de um payout vêm do{" "}
            <strong>workflow do cash-loop</strong> (e a competência/valor do pró-labore, do employment-clt); a EFI só{" "}
            <strong>executa o Pix</strong>. Aqui o papel é <strong>confirmar + observar</strong> — não originar.
          </p>
          <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.55 }}>
            O <strong>histórico de payouts / pró-labore</strong> não é lido por esta surface — não há op{" "}
            <code>observe_payouts</code> no adapter (<em>needs-work</em>). Não inventamos uma lista de repasses.
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

        {/* money-movement: create_payout — admin-tier, gated + disabled, refused in homolog */}
        <GatedAction
          need="efi.payouts.create"
          perms={scope.perms}
          env={env}
          eyebrow="Remediação"
          label="Disparar um payout (Pix de saída) é movimentação de dinheiro — admin, fora da v1. Quando o caminho de escrita for ligado, ela apenas confirma um payout que o cash-loop já decidiu (idempotente + auditável), nunca origina o valor/destino."
          hint="create_payout é admin, fora da v1, e recusado enquanto homolog."
        />
      </div>
    );
  }

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Payouts & Prólabore"
        subtitle="Histórico de payouts é needs-work (sem op de leitura). A surface NUNCA decide quem/quanto pagar — o cash-loop decide; aqui é confirm + observe. Mover dinheiro é admin e recusado em homolog."
        teamChips={<EnvironmentBadge env={env} size="sm" />}
        kpis={instanceLoading ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
