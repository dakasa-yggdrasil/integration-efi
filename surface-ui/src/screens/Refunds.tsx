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
        {/* rule-#0 reminder — refund is ops remediation, not a product button */}
        <p style={{ margin: 0, fontSize: "var(--fs-sm)", color: "var(--mut)", lineHeight: 1.5 }}>
          Devolução = remediação de ops. Histórico de devoluções: sem op de leitura.
        </p>

        {/* money-movement: refund_charge — admin-tier, gated + disabled, refused in homolog */}
        <GatedAction
          need="efi.refunds.create"
          perms={scope.perms}
          env={env}
          eyebrow="Remediação"
          label="Estornar charge (devolução Pix) — admin, em breve."
          hint="refund_charge é admin e recusado em homolog."
        />
      </div>
    );
  }

  return (
    <div className="atelier" style={SHELL_WRAP}>
      <TierTwoShell
        eyebrow="Conta"
        title="Refunds"
        subtitle="Remediação de ops — admin, em breve."
        teamChips={<EnvironmentBadge env={env} size="sm" />}
        kpis={instanceLoading ? undefined : kpis}
      >
        {body()}
      </TierTwoShell>
    </div>
  );
}
