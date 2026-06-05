# Design note: declaring byte order on the struct itself

**Status:** Decided (round 2) — ready to implement; see Decisions D1–D6 and
Resolutions V1–V5. Not yet implemented.
**Relates to:** the v0.3.0 change that moved byte order onto the `Marshaler`
(`NewMarshaler(order)`), and the existing per-field `endian=` tag.
**Author/reviewers:** (add your name / comments inline — see *Reviewer comments* at the end.)

> How to comment: leave inline notes as `<!-- jc: ... -->` or fill the **Reviewer
> comments** section. Decision points are marked **❓ OPEN**.

---

## 1. Problem

A binary format's byte order is almost always **fixed and intrinsic to the
format**: a PNG header is big-endian, a ZIP local header is little-endian. Today
that fact lives *outside* the type — it is passed at call time
(`NewMarshaler(order)` / `binarystruct.Marshal(v, order)`). Two consequences:

- The single most stable property of the format is the one thing **not**
  documented next to the struct that describes the format.
- A caller can pass the wrong order and silently produce a malformed blob; the
  type cannot defend itself.

We already attach per-member attributes with `binary:"..."` tags. The question is
whether we can attach this **struct-level** fact to the struct too.

## 2. The Go constraint

Go has **no struct-level tags** — tags are syntactically attached to *fields*
only. So any struct-wide metadata must be carried by one of a handful of
workarounds. Candidates:

| # | Mechanism | Example | Verdict |
| :- | :-- | :-- | :-- |
| A | **Blank sentinel field + tag** | `_ struct{} \`binary:"endian=big"\`` | **Recommended** |
| B | Method / interface | `func (Packet) BinaryByteOrder() ByteOrder` | Runner-up |
| C | Embedded marker type | embed `binarystruct.BigEndianStruct` | Rejected |
| D | Registration | `binarystruct.Register(Packet{}, BigEndian)` | Rejected |

- **C (embedded marker)** promotes methods into the struct's method set, can only
  express big/little (not `inverse` or future options), and is rigid. Rejected.
- **D (registration)** is not co-located with the struct, so it drifts and an
  agent reading the type never finds it. Rejected — it defeats the goal.

So the real choice is **A vs B**.

## 3. Recommendation: the blank sentinel field (A)

```go
type PNGHeader struct {
    _     struct{} `binary:"endian=big"` // declares this format's byte order
    Magic [8]byte  `binary:"[8]byte,const=0x89504e470d0a1a0a"`
    Width uint32   `binary:"uint32"`
    // ...
}
```

The zero-size blank field carries struct-scope options in the same `binary:` tag
grammar already used everywhere else; it encodes to **0 bytes** and is purely
metadata.

### Why A over B (the method) for *this* library

1. **One configuration surface.** Everything is already configured through
   `binary:` tags. Agents and humans learn one mechanism; a struct-level
   `endian=` is just the existing option promoted to struct scope. The parser
   already parses `endian=`.
2. **Codegen parity / zero drift — decisive.** `binarystruct-codegen` reads tags
   from the AST, so a sentinel tag is parsed statically and baked in trivially. A
   *method's* return value cannot be reliably evaluated at generation time, so B
   would split the truth (runtime reads the method; codegen reads the `-endian`
   flag) — precisely the drift this project exists to prevent.
3. **Self-documenting & co-located.** It is the first line of the struct, visible
   in the source and in godoc — exactly where the format's reader looks.
4. **Additive & backward-compatible.** A struct without the sentinel behaves
   exactly as today.

B (the method) is more type-safe and is idiomatic Go (mirrors `encoding.*`), and
is a defensible choice — but the codegen-drift problem is what tips the decision
to A.

## 4. Semantics

### 4.1 Precedence (the key decision)

Order is resolved by **decreasing specificity**:

```
per-field endian=   >   struct-level endian=   >   Marshaler order (NewMarshaler/arg)   >   error
```

**❓ OPEN #1 — does a struct-declared order win over the Marshaler's order?**
My recommendation: **yes.** A PNG header *is* big-endian; that is intrinsic to the
format, not a caller's choice. So `NewMarshaler(LittleEndian).Marshal(&pngHeader)`
should still emit **big-endian**, and the Marshaler's order governs only types
that do **not** declare one (and bare scalars / `…As` calls).

- *Upside (pillar 5):* a whole class of "I passed the wrong order" bugs
  disappears — the type carries its own truth.
- *Downside:* the (rare) surprise that an explicit caller order is ignored for a
  declared type. Must be documented loudly.
- *Alternative view to weigh:* treat the struct order as a **default** that the
  caller's explicit order overrides. More flexible, less safe. (I lean against.)

### 4.2 Nested structs and `inverse`

Consistent with today's field-level `endian=`:
- A nested struct with its **own** struct-level `endian=` overrides the inherited
  order for its subtree.
- A nested struct **without** one inherits the effective order from its parent.
- `endian=inverse` at struct level flips relative to the inherited order, exactly
  as `resolveByteOrder` already does for fields.

### 4.3 Bare scalars / `MarshalAs` / non-struct values

These have no struct to carry a sentinel, so they continue to use the Marshaler's
order (or the `…As` call's). Unchanged.

### 4.4 No order anywhere

If a value neither declares a struct-level order nor is given one by the
Marshaler, behavior is the **fail-loud** error introduced in v0.3.0
("no byte order set …"). Unchanged.

## 5. Implementation sketch (three paths must stay in sync)

Per the `SPECIFICATION.md` §3 extension protocol:

- **Parser (`struct.go`, `getStructMetadata`)**: recognize a blank `_` field
  whose `binary:` tag carries struct-scope options; parse `endian=` into a
  struct-level order stored on the struct metadata; ensure the sentinel field is
  excluded from the field layout (0 bytes, never encoded/decoded).
- **Safe runtime (`marshal.go`/`unmarshal.go`)**: when entering a struct
  (`writeStruct`/`readStruct`), seed the effective order from the struct-level
  order if present, *before* the existing per-field `resolveByteOrder(order,
  field.endian)`.
- **Unsafe runtime (`unsafe_io.go`)**: same seeding in
  `unsafeWriteStruct`/`unsafeReadStruct`.
- **Codegen (`binarystruct-codegen/generator.go`)**: read the struct-level
  `endian=` at generation time and emit the struct's `WriteBinary`/`ReadBinary`
  using that order as the base. (Interaction with the `-endian` flag: see OPEN #3.)
- **Tests**: round-trip in all three modes; nested override; precedence vs a
  conflicting Marshaler order; the sentinel contributes 0 bytes.
- **Docs**: `STRUCT_TAGS.md` (+ `_ja`), `SPECIFICATION.md`, `llms-full.txt`,
  README recipe.

## 6. Tag syntax details

- Carrier: a blank field `_ struct{}` (zero size). `_ [0]byte` would also work;
  `struct{}` is the idiomatic zero-size type.
- Only **struct-scope** options are valid in the sentinel tag. For now that is
  `endian=`. The grammar leaves room for future struct-wide defaults.

**❓ OPEN #2 — should the sentinel also carry other struct-wide defaults later**
(e.g. a struct-level default text `encoding=`)? Not needed now, but the mechanism
generalizes; deciding the scope now avoids re-litigating the carrier later.

**❓ OPEN #3 — codegen `-endian` interaction.** Today `-endian` is *required* and
bakes the order into the no-arg `MarshalBinary`/`AppendBinary`/`UnmarshalBinary`.
If a struct declares its own order, options:
  (a) the struct-level order wins and `-endian` becomes optional/ignored for
      declared types (consistent with §4.1 precedence); or
  (b) `-endian` still sets the *no-arg method* default, while the struct-level
      order governs the field encoding — they address different layers.
My lean: **(a)** for declared types, to keep one source of truth.

**❓ OPEN #4 — discoverability of `_ struct{}`.** The blank-field idiom is real
but slightly obscure. Mitigation is a clear recipe in `llms-full.txt` + README.
Alternative carriers (a named marker type, a non-blank reserved field name) trade
clarity for more surface. Worth a quick opinion.

## 7. Backward compatibility

Fully additive. Existing structs (no sentinel) behave exactly as in v0.3.0. This
could ship in a minor release (e.g. v0.4.0) with no further breakage.

## 8. Examples

```go
// Big-endian network format — order declared once, on the type.
type PNGChunkHeader struct {
    _      struct{} `binary:"endian=big"`
    Length uint32   `binary:"uint32"`
    Type   [4]byte  `binary:"[4]byte"`
}

// Little-endian on-disk format.
type ZIPLocalHeader struct {
    _         struct{} `binary:"endian=little"`
    Signature uint32   `binary:"uint32,const=0x04034b50"`
    Version   uint16   `binary:"uint16"`
}

// Mixed: struct is little-endian, one field overrides to big.
type Mixed struct {
    _  struct{} `binary:"endian=little"`
    LE uint32   `binary:"uint32"`            // little (struct default)
    BE uint32   `binary:"uint32,endian=big"` // field override wins
}
```

With these, `binarystruct.Marshal(&pngChunk, …)` produces big-endian regardless
of the order argument (per OPEN #1), and the order no longer has to be remembered
at every call site for a fixed-format type.

---

## Reviewer comments

<!-- Add notes here (or inline as `<!-- jc: ... -->`). -->

- **OPEN #1 (struct order wins vs default):**
Struct order wins: we may embed structs to other structs, and the endian may automatically follow the embedded truth.
Moreover, we should remove `order` argument from standard Marshal() / Unmarshal() functions and NewMarshaller()/NewUnmarshaller() to encourage and simplify the specification (and maybe add MarshalOrder() / UnmarshalOrder() for compatibility). 

- **OPEN #2 (other struct-wide defaults):**
Yes it's good to have other struct-level defaults. Default string encoder looks useful.

- **OPEN #3 (codegen -endian interaction):**
If structs are tagged correct, then we may remove `-endian` argument from codegen. But we should emilt a warning if no endian is designated in both command-line and struct.

- **OPEN #4 (sentinel discoverability / carrier choice):**
Write a clear document to encourage to specifiy the endian (only) in the TOP-level struct. Also, update examples including "A Quick Example" section to show the patten.

- **Other:**

---

## Post-review resolution (round 1)

Decisions taken from the review, and the new questions they open. **Decisions are
locked unless re-opened; V-items below need a call before implementation.**

### Decisions

- **D1. Struct-declared order wins** over any Marshaler/argument order (OPEN #1).
- **D2. Order becomes primarily a struct property.** Remove `order` from the
  standard `Marshal`/`Unmarshal`/`Write`/`Read`/`Append` and from the
  `NewMarshaler` constructor; the struct's declaration supplies it. Provide an
  explicit-order **escape hatch** for values that cannot declare one (bare
  scalars, third-party structs, `…As`). (OPEN #1.)
- **D3. The sentinel carrier is general**, not endian-only; the next struct-level
  option is a **default text `encoding=`** (mirrors the existing
  `Marshaler.DefaultTextEncoding`). (OPEN #2.)
- **D4. Embedding propagates a declared order.** Embedding a struct that declares
  an order gives the embedder that order (unless the embedder declares its own).
  This enables reusable named bases, e.g. `type bigEndian struct{ _ struct{}
  \`binary:"endian=big"\` }` embedded into format types. (OPEN #1.)
- **D5. Codegen `-endian` becomes optional** (struct declaration wins). If neither
  the struct nor `-endian` specifies an order, **generation fails with a clear
  error** (fail-loud, consistent with the runtime; supersedes the earlier "warn"
  idea — see V5). (OPEN #3.)
- **D6. Documentation**: encourage declaring endian on the **top-level** struct
  only (nested/embedded inherit); update `STRUCT_TAGS`(+ja), `SPECIFICATION`,
  `llms-full.txt`, README, and the godoc/README **"A Quick Example"** to show the
  sentinel. (OPEN #4.)

### Resolutions (round 2)

- **V1 → DECIDED: fold into the unreleased 0.3.0.** Ship the order-on-struct API
  once; do not release `NewMarshaler(order)` and deprecate it.
- **V2 → DECIDED: instance-based escape hatch.** `NewMarshalerOrder(order)` (and
  the `Marshaler{Order: order}` field) supplies an explicit order for values that
  don't declare one; the order-free methods use it as the fallback. No per-verb
  `…Order` package functions.
- **V3 → DECIDED: keep fail-loud.** Undeclared value + no escape-hatch order → the
  v0.3.0 "no byte order set" error (no implicit default).
- **V4 → DECIDED: error at metadata time** when multiple embedded structs declare
  conflicting orders (no silent first-wins).
- **V5 → DECIDED: codegen fails with a clear error** when neither the struct nor
  `-endian` gives an order (no omitted methods, no fabricated default).

### Original open questions (now resolved above)

- **❓ V1 — fold into the *unreleased* 0.3.0, don't ship-then-change.** 0.3.0 is
  not tagged/released (feature branch only). D2 reverses the very `NewMarshaler(order)`
  / `Marshal(v, order)` shape 0.3.0 just introduced. **Recommendation: revise the
  0.3.0 commits so we ship the order-on-struct API once**, rather than releasing
  `NewMarshaler(order)` and immediately deprecating it. (Alternative: ship 0.3.0
  as-is, do this as 0.4.0 — more churn for users.)
- **❓ V2 — shape of the explicit-order escape hatch.** Options:
  (a) package funcs per verb: `MarshalOrder(v, order)`, `UnmarshalOrder(data, order, v)`,
      `WriteOrder`, `ReadOrder`, `AppendOrder` — complete but five new names;
  (b) **instance-based**: `NewMarshalerOrder(order)` (or `Marshaler{Order: order}`),
      then the order-free methods use it as the fallback — one escape hatch for all
      verbs. **Recommendation: (b)**, plus the two common conveniences
      `MarshalOrder`/`UnmarshalOrder` if you still want them. (Confirms the wording
      from OPEN #1's "maybe add MarshalOrder()/UnmarshalOrder()".)
- **❓ V3 — undeclared + no explicit order.** A value with no struct-declared order,
  marshalled via the order-free API with no escape-hatch order, keeps the v0.3.0
  **fail-loud** error. Confirm (vs. defaulting to big-endian).
- **❓ V4 — embedding conflict resolution.** Embedder's own sentinel wins; else an
  embedded struct's declared order. If **multiple embedded structs declare
  conflicting orders**: error at metadata time (recommended) vs. first-wins.
  Confirm.
- **❓ V5 — codegen warning, then what order?** When neither `-endian` nor the
  struct gives an order, after warning, the no-arg `MarshalBinary`/`AppendBinary`/
  `UnmarshalBinary` still need *some* baked order. Options: (a) **omit** those
  no-arg stdlib methods for that type (still emit `WriteBinary(w, order)`), or
  (b) bake a documented default (big-endian). **Recommendation: (a)** — don't
  fabricate an order the format never stated.

### Note on `Unmarshaler`/`NewUnmarshaller`

The review mentioned `NewUnmarshaller()`; for the record there is **one** type,
`Marshaler`, that does both directions (no separate `Unmarshaler`). The
escape-hatch constructor would be a single `NewMarshalerOrder(order)` used for
both encode and decode.
