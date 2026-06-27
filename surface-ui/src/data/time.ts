// Format an EFI `created` value into a terse pt-BR absolute timestamp for the
// charges roster, plus a short relative form. UNLIKE Stripe (Unix epoch
// seconds), EFI returns `created` as an RFC3339 string
// ("2026-05-10T12:00:00Z"). These are config-grade reconciliation facts (when a
// charge was created), never anything payer-identifying.

/** Parse an RFC3339 string to a Date, or null when empty / unparseable. */
function parse(created: string): Date | null {
  const s = (created ?? "").trim();
  if (s === "") return null;
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** "10/05 12:00" — date + time, no year (the roster is recent charges). */
export function formatCreated(created: string): string {
  const d = parse(created);
  if (!d) return "—";
  return new Intl.DateTimeFormat("pt-BR", {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit"
  }).format(d);
}

/** "há 7 min" / "há 2 h" / "há 3 d" — a relative hint, or "" when unknown. */
export function relativeCreated(created: string): string {
  const d = parse(created);
  if (!d) return "";
  const deltaSec = Math.max(0, Math.floor((Date.now() - d.getTime()) / 1000));
  if (deltaSec < 60) return "agora";
  const min = Math.floor(deltaSec / 60);
  if (min < 60) return `há ${min} min`;
  const hours = Math.floor(min / 60);
  if (hours < 24) return `há ${hours} h`;
  const days = Math.floor(hours / 24);
  return `há ${days} d`;
}
