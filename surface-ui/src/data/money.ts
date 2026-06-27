// Currency formatting for EFI Pix. UNLIKE Stripe (which sends amounts in the
// SMALLEST unit / cents), EFI returns `valor` as a decimal REAIS string in the
// MAJOR unit — e.g. "150.00" means R$ 150,00 and "250.50" means R$ 250,50. So
// we parse the decimal string to a number of reais and format it with Intl, in
// pt-BR, as BRL. (EFI Pix is BRL-only; there is no multi-currency case here.)

/** Parse an EFI `valor` decimal-reais string ("150.00") to a number of reais. */
export function parseReais(valor: string | number | null | undefined): number {
  if (typeof valor === "number") {
    return Number.isFinite(valor) ? valor : 0;
  }
  const s = (valor ?? "").toString().trim();
  if (s === "") return 0;
  // EFI uses a dot decimal separator ("150.00"); be defensive about a stray
  // comma form ("150,00") in case an upstream flow differs.
  const n = Number(s.includes(",") && !s.includes(".") ? s.replace(",", ".") : s);
  return Number.isFinite(n) ? n : 0;
}

/**
 * Format an EFI reais amount to BRL, e.g. `formatMoney("48200.00")` →
 * "R$ 48.200,00". Accepts the raw `valor` string (or a number of reais). Uses
 * the pt-BR locale so grouping/decimal separators match the rest of the
 * console.
 */
export function formatMoney(valor: string | number): string {
  const reais = parseReais(valor);
  return new Intl.NumberFormat("pt-BR", {
    style: "currency",
    currency: "BRL",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(reais);
}

/** Sum a list of EFI charge `valor` strings into a total number of reais. */
export function sumReais(valores: Array<string | number>): number {
  return valores.reduce<number>((acc, v) => acc + parseReais(v), 0);
}
