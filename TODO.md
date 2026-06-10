# TODO List

> Shipped work is recorded in [CHANGELOG.md](CHANGELOG.md) (per release) and git
> history; this file tracks only what is **not done yet**. Rationale for the
> deliberate codegen exclusions lives in [AGENTS.txt](AGENTS.txt) §1.

## Pending / Future Ideas
- [ ] **Batch contiguous fixed-width scalar fields (codegen perf)**: codegen emits one `w.Write(tmp[:N])` per fixed-width scalar field, plus a per-method `var tmp [8]byte` that escapes through the `io.Writer` interface (≈1 alloc per generated method call, multiplying under nesting). Batching runs of contiguous fixed-width fields into a single buffer + one `Write`/`ReadFull` (like the array bulk path, and like `encoding/binary`'s fixed-size path) would cut both the alloc and the N calls; the generator change must track runs across mixed field types. **Codegen-only** now — the runtime per-scalar heap escape was already removed in 0.3.2 via the reusable `ms.scratch` buffer. (Surfaced by the 0.3.2 codegen profiling pass.)
- [ ] **Optional strict arg-name check for custom `valueof` evaluators (low priority)**: a custom `valueof=CRC32(Type, Data)` passes its referenced fields positionally as `ctx.Args`; transposing the arg names yields a different-but-self-consistent result (encode and decode use the same order) with no compile-time or run-time check. Consider an opt-in mode where an evaluator can assert the arg names it expects (e.g. via `ValueOfContext.Args[i].Name`), catching transposition. Inherent to the positional design; surfaced by the 0.3.2 clean-agent eval as low-severity.

## Deferred — the runtime interpreter is the intended answer
These codegen feature gaps **fail loud and fall back to the runtime interpreter by
design** (see [AGENTS.txt](AGENTS.txt) §1, "Deliberate codegen exclusions"). They
are not planned work unless a concrete need arises — the runtime handles every case:
- **Codegen multidimensional arrays over non-scalar leaves**: scalar-leaf multidim is generated; string / nested-struct / pointer leaves and mixed fixed-array/slice nesting stay on the runtime. Supporting them would need the leaf emitter to handle non-scalar element types inside the nested loops.
- **Codegen custom `valueof` over nested-struct args**: the one unsupported arg shape (all others are emitted inline or re-encoded via `ms.MarshalAs`). Would need a fully-static emit of the nested struct into a scratch buffer (its own byte-order resolution included), which the current `ms.MarshalAs` reuse cannot express in a standalone tag.
