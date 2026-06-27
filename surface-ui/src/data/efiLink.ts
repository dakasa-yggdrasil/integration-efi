// Deep-link helpers to the NATIVE EFI (sejaefi.com.br) console. RULE #0: this
// surface is a finance-OPS view — it shows config / health / reconciliation refs,
// never a customer Pix/billing view and never money-movement controls. Anything
// that means "act on this in EFI" (inspect a charge, run a refund, register a
// webhook) is a deep-link ("↗") OUT to the real EFI console, never an in-surface
// money action.
//
// HONESTY about the base URL: the surface-query responses do NOT carry the
// instance's EFI console host (the adapter shapes only row fields). Under
// `?mock` we supply the real console host (MOCK_EFI_BASE). In the live path the
// host is not yet wired through a surface read, so {@link useEfiBase} returns ""
// and the UI degrades honestly: a deep-link with an unknown base is rendered
// DISABLED with a tooltip — we never point "↗" at a guessed URL.

/** Normalize an EFI console base into a host root with no trailing slash. */
export function normalizeEfiBase(raw: string | undefined): string {
  let base = (raw ?? "").trim();
  if (base === "") return "";
  base = base.replace(/\/+$/, "");
  return base;
}

/** The native EFI Pix area (where charges/extrato live), or "". */
export function pixHref(base: string): string {
  const host = normalizeEfiBase(base);
  if (host === "") return "";
  return `${host}/pix`;
}

/** The native EFI webhook settings area, or "". */
export function webhookHref(base: string): string {
  const host = normalizeEfiBase(base);
  if (host === "") return "";
  return `${host}/pix/webhook`;
}
