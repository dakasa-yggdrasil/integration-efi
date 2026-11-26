# Fixed CPU capacity observation

`/app/integration-efi --print-cpu-capacity-runtime` is a separate read-only child
mode. It exits after one bounded JSON line. It does not initialize the logger,
SDK, provider credentials, transport or confidential receiver. Extra arguments,
path/PID options and combined modes are rejected before any startup or read.

The existing `--print-confidential-runtime` five-field DTO and the no-argument
startup remain unchanged. This mode does not publish or restore a receiver key.

The closed version1 result contains exactly these fields:

```json
{
  "schemaVersion": 1,
  "pid1StartTicks": 73119,
  "sameCgroup": true,
  "cpuQuotaMicros": 200000,
  "cpuPeriodMicros": 100000,
  "cpuUnlimited": false,
  "memoryMaxBytes": 536870912,
  "memoryUnlimited": false
}
```

The values above are examples, not observations of production. The kernel's
`max` is represented by a null quota/max and its corresponding unlimited flag
set to true. An observation of an unlimited value is not a finite resize policy.
No cgroup path, process command, environment, credential or key field is emitted.

The reader opens only `/proc/1/stat`, `/proc/self/cgroup`, `/proc/1/cgroup`,
`/sys/fs/cgroup/cpu.max` and `/sys/fs/cgroup/memory.max`. It requires Linux,
regular bounded files and actual cgroup-v2 file descriptors for the limit files.
Both processes must report the single unified namespace-root cgroup `0::/`.
Non-root, mismatched, v1 and hybrid hierarchies are unsupported: fixed root files
must never be mistaken for this container's limits in another hierarchy.

Two observations must agree on PID1 start ticks, quota, period and memory max.
PID1 must be live in a supported running/sleeping state; stopped, exiting,
unavailable, malformed or changing observations return exit1 with only the
closed diagnostic `cpu capacity runtime unavailable`. A wrong argument set
returns exit2. Neither failure initializes another adapter or receiver.

This is an observation ABI only. It does not add a controller, Role, resize,
replica change or automatic capacity behavior. A future owner must independently
bind the exact Pod UID/container CID/start/restart count and preserve memory,
credentials and the current confidential lifetime. PID1 start ticks alone are
not a Kubernetes lifetime binding or proof of an original request's completion.

A new image rollout still rotates the RAM-only confidential receiver. Receive
old work and normal SDK/process shutdown, then qualify the new public recipient
before any adoption. There is no shared/restored old key, automatic resend,
throughput/DAU claim or current production eligibility claim in this Source change.
Parser, fixed dispatch, race, privacy and unsafe-file checks run in GitHub Actions;
no tests or product binary are run locally.
