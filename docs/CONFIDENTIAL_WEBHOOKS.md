# Confidential webhook observations and reconciliation

Adapter source version 2.5.7 adds optional confidentiality to the existing
`observe_webhook_subscriptions` and `ensure_webhook_subscription` actions. It
adds no action, credential, permission, provider API or generic SDK transport.
The SDK remains v0.9.1. Existing plaintext callers retain their legacy behavior.
Source version alone does not establish publication, qualification or deployment.

## Protect both directions

An asynchronous orchestrator can persist its integration inputs, broker messages,
results and events. Marking a Pix input sensitive does not protect other account
keys returned by a complete LIST. Confidential callers therefore encrypt ALL
private provider parameters before HTTP/Core/broker delivery and receive the
complete legacy output only inside an encrypted result. Neither plaintext
parameters nor raw provider records may accompany the sealed envelope.

The execute `input` has exactly this single member:

```json
{
  "confidential_input": {
    "schemaVersion": 1,
    "algorithm": "nacl-box-seal-anonymous-v1",
    "recipientKeyID": "<current-runtime-key-UUID>",
    "ciphertext": "<canonical-standard-base64>"
  }
}
```

This illustrates the shape, not a runnable ciphertext. Authoritative wire types
are [types.go](../providers/efi/confidential/types.go); standard NaCl anonymous
box encoding, strict JSON, limits, intent hashing and HMAC framing are in
[codec.go](../providers/efi/confidential/codec.go). Unknown typed fields, omitted
required zero/false members, wrong field case, duplicate JSON members and
noncanonical base64 fail closed. Public keys must have exactly 32 bytes and pass
the low-order X25519 check.

Sealed input includes the private Pix comparison key, canonical receiver base, full creation
window/page parameters, a result-recipient public key and a per-invocation
comparison secret. Its binding includes release/source claims, invocation,
workflow/version, step/operation/purpose, integration instance, receiver/result
key fingerprints, comparison-secret fingerprint, audit UUID and authorization
times. The adapter verifies actual operation/instance/runtime fields. Core
Metadata does not authenticate caller-supplied release or workflow claims; the
operator must compare every decrypted claim to its original immutable attempt.

`intent_sha256` hashes the compact sorted Go JSON encoding of `Intent`, without
a trailing LF. It excludes itself and the later ciphertext/wire hash, avoiding a
hash cycle. This is distinct from the final webhook B7 digest.

## Process recipient and fixed public reader

The main adapter process generates its incoming X25519 private key in RAM. It
never writes that key to a file, environment, log, event or response. A process
restart rotates it; an old encrypted request then fails before provider access.

The only runtime file is `/run/efi-confidential/runtime.json`, containing a closed
public DTO of at most 4KiB: schemaVersion, algorithm, adapterVersion, keyID and
publicKey. Startup atomically replaces stale metadata before readiness can
succeed. Missing/invalid metadata prevents startup/readiness. The reader rejects
symlinks, nonregular files, changed files and oversized content.

The distroless image's fixed reader is:

```text
/app/integration-efi --print-confidential-runtime
```

This exact sole argument branches before logging, SDK startup, credential/TLS
loading or key generation. It only prints the validated public DTO. There is no
caller path, arbitrary executable, shell, credential read or repair mode.

A production deployment must provide the small dedicated **public-only** memory
emptyDir at `/run/efi-confidential`, writable by the existing nonroot UID/GID 65532
with the required group ownership. Preserve readOnlyRootFilesystem, distroless
and one container. This repository does not claim that a caller's deployment
profile already contains that mount. No private-key file or secret volume is
introduced by this mode.

The operator must bind the qualified image/source, complete owned population,
DeploymentUID/generation, PodUID, containerID/restartCount and recipient keyID/key
fingerprint, then reread physical population and public metadata before POST.
Missing, stale or changed identity holds. A public key is not proof of runtime
identity by itself. NaCl anonymous boxes protect confidentiality/integrity but
do not authenticate a sender; existing authenticated source/routing/RBAC proofs
remain necessary.

The outgoing result key and comparison secret belong to the operator's original
invocation. This adapter stores neither persistently. An operator missing its
original private result key must hold; it must not rekey/replay a provider action
to recover an original result.

## Exact equality without public account keys

For each invocation generate an independent random 32-byte comparison secret K.
Only the operator's private task key store and sealed adapter inputs contain K.
The public projection contains `key_tag`/`url_tag` lowerhex64 HMAC-SHA256 values,
page integers/provenance and validation booleans, with no raw account key/URL or
unknown provider field. Root independently computes expected target tags from
the real target key and fixed canonical URL in RAM; Core compares those exact
strings instead of trusting adapter-provided match booleans.

For HMAC framing concatenate three fields, each prefixed by its uint32 big-endian
UTF-8 byte length: domain, canonical invocation UUID, exact string. Domains are
`efi.webhook.confidential.key.v1` and `efi.webhook.confidential.url.v1`. Step and
operation are absent from the HMAC frame so one invocation's LIST/single/ensure/
confirm can use the same expected tags; they remain bound separately inside each
sealed message. A new ensure invocation uses a new K, not the prior observer's
tags. Empty/malformed record fields never become a tag for a valid record.

Tags use the exact string in the emitted legacy DTO. In particular, existing
webhook output redacts query **values** while keeping query names: tags cover
that exact emitted `webhookUrl`, without parsing equivalence, trimming, case
folding or further normalization. Original URL safety/page validation still
happens before sanitization. The canonical base has no query, so query-bearing
registrations remain noncanonical. Do not claim the operator can recompute a tag
over pre-sanitization query values absent from its legacy DTO.

Tags reveal row equality, URL grouping and target matching within the invocation.
They are ephemeral pseudonyms, not anonymity. Independent K prevents simple
cross-invocation linkage and offline CPF/email dictionary tests without K under
the standard HMAC-SHA256 PRF/collision assumptions.

The operator decrypts the complete original legacy DTO in RAM and independently
recomputes EVERY projected tag, row count/order, alias, date and classification,
as well as the full original pagination/URL/UUID checks. The encrypted output
preserves unknown nested provider fields and aliases; safe projection is not a
replacement for those independent checks.

## Fresh ensure, actual provider validation and held outcomes

The orchestrator keeps its own complete LIST/assert chain. Confidential Ensure
also proves its own fresh full-window LIST before mutator use. It accepts only
one authoritative complete page, exact window/page/size echoes and all valid
unique records. A cursor alone, missing/malformed metadata, multiple pages,
provider 400/403/404 or a foreign same-key registration never establishes absence.
There is no pagination loop or narrower-window fallback.

A complete LIST may prove absence or canonical same-key state. Either case
requires the actual setter with `skip_mtls_validation:false`, so EFI performs
its provider-side mTLS validation in this invocation. Canonical state or an
earlier GET cannot waive that provider check. The existing setter uses
`PUT /v2/webhook/{chave}`, with its existing `/v3/gn/webhook/{chave}` fallback on
v2 HTTP404. Confirmed proof and sealed legacy output both report `noop:false`;
endpoint is the actual setter route `v2` or `v3`, never a URL or fabricated GET
route. A final exact canonical GET is still required.

The binding requires canonical UTC issuedAt/expiresAt, issuedAt at most 30 seconds
ahead, expiresAt after issuedAt and at most 5 minutes later. The operator additionally
bounds ensure expiry by the original successful observe FinishedAt + 5 minutes and
any explicit task lease expiry. Check delivery authorization and check again
immediately before EVERY actual PUT attempt, including v3 fallback; no expiry
grace applies. This guards the local send boundary, not remote arrival time or
an external lease absent from the contract. Each authorized PUT also has a child
HTTP request deadline bounded by the remaining authorization; cancellation of
that child leaves the original confirmation/sealing/event context intact. This
is an authorized local HTTP attempt/request-deadline guarantee, not proof of the
time physical bytes arrive or a remote commit happens. Maintain the exclusive
writer window through terminal return or hold. No provider CAS/atomic ownership
is claimed.

After a PUT has been accepted, bounded read-only confirmation, sealing and event
attempts may finish after its authorization deadline. A separately bound
`purpose:confirm` permits that late exact GET; original-run GET/decryption is also
allowed after expiry. Neither is authority for another PUT or refreshed expiry.

The SDK outer success envelope alone is insufficient. A mandatory Core assert
must require **`proof.outcome == "confirmed"` AND `proof.ensured == true` AND
`proof.noop == false`** before
continuing an ensure workflow toward successful evidence. If HTTP 2xx PUT headers
were received but response read/size/decode, later confirmation or sealing fails,
the adapter returns a bounded safe `proof.outcome:"held"`, `ensured:false`
Observed outcome, possibly with empty ciphertext. This preserves the SDK's
best-effort mutation-event attempt while refusing success/B7. A PUT transport
error is unknown, not invented confirmation. No plaintext fallback is allowed.

SDK ensure events represent reconciled desired state, not a physical exactly-once
receipt. Confirmed confidential state requires an actual accepted setter and
readback. Opaque audit UUID replaces raw Pix identity in confidential events;
Observed remains sealed/safe before SDK emission. Event delivery remains the
existing best-effort contract. An audit UUID equal to the requested key or any
own-LIST provider key is refused before mutation/event output.

One Root async POST/logical step is not physical exactly-once. SDK reply loss can
redeliver a handler, which proves another fresh LIST and may repeat the actual
setter within the same authorization interval, including for canonical state.
There is no RAM cache that skips the provider validation. One logical setter
operation may also include the bounded v2/v3 route fallback. Unknown outcomes
remain held and recover only through the original run's GET; Root must not resend
a POST, rotate keys or renew an immutable invocation.

## Size, sinks and evidence

Complete request plaintext is limited to 8KiB, complete sealed result plaintext
to 512KiB, including binding and internal guard observations. Confidential provider
body reads reject max+1 overflow before decode. Never truncate, partially prove,
or return raw data when the sealed result is too large. The operator's aggregate
getter must also accommodate the entire actual workflow result; the Root client
has an 8MiB cap. Four encrypted outputs plus foreach/assert metadata require an
actual serialized-size CI check, not a three-output estimate.

Confidential errors become static before efiapi tracing/RPC logging, preserving
only safe HTTP status for the existing fallback. SDK event-emitter failures are
also masked before warnings. Counts, timing, ciphertext length and ephemeral task
correlation remain visible. This is not a guarantee against privileged memory
inspection, kernel swap or crash dumps.

B7 remains SHA256 of the bytes `jq -cS .` prints for the COMPLETE actual plain
final-confirm legacy output, including jq's final LF. Decrypt in RAM and use that
whole `legacy_output`; never hash the cipher envelope, projection, selected
fields, intent encoding or an adapter-supplied digest. Do not save decrypted
results or private keys to repository/temporary files or logs.

Synthetic interoperability inputs are in
[interop-v1.json](../providers/efi/confidential/testdata/interop-v1.json).
Its labelled canonicalization placeholders are not valid sealed requests;
standard cryptographic outputs are derived only when the consuming CI tests run.
No provider future-window acceptance, published image, deployed identity or
passing CI is asserted by this document.
