# PACE

PACE (Progressive Augmented Compiler Extensions) is an independent compiler/toolchain overlay for the Go programming language. It adds opt-in low-level compiler extensions while preserving stock-Go source compatibility and a small, easily rebased patch stack over upstream Go.

**PACE is an independent project and is not affiliated with, sponsored by or endorsed by Google LLC or the Go project.**

## Extensions

* `//go:inline` forces eligible functions to be inlined regardless of normal cost heuristics; ordinary inlining eligibility restrictions still apply.
* `//go:linkinternal` allows functions to inherit the compiler intrinsic behavior of internal Go functions while retaining a standard-Go fallback.
* `//go:abiinternal` allows ordinary Plan 9 assembly functions to use Go's internal register ABI with an explicit argument and result register mapping on **amd64, arm64, loong64, ppc64, ppc64le, riscv64 and s390x**.

PACE intentionally defines no separate language syntax. Extensions use otherwise-ignored `//go:` directives:

```text
same source
  stock Go -> directives are ignored and fallback behavior remains valid
  PACE     -> directives activate the enhanced compiler behavior
```

Keep valid fallback implementations and architecture-appropriate build constraints in your source.

### Assembly register mappings

`//go:abiinternal` applies to a bodyless function with an assembly definition. Every parameter and result must be named and mapped exactly once, with exactly one `->` separating inputs from results:

```go
//go:abiinternal addr=DX new=BX width=CX signed=DI -> old=AX
func SwapIfLessInteger(addr *uint64, new uint64, width uint8, signed bool) (old uint64)
```

Keep the assembly in ordinary stock-compatible form, including its ABI0 argument size and named `FP` loads/stores: `TEXT .SwapIfLessInteger(SB), NOSPLIT, $0-32`. Stock Go ignores the directive and uses ABI0 assembly and its usual wrapper. PACE emits the definition as ABIInternal, maps each named `FP` operand to its requested register and removes mapped identity integer moves (`MOVB`, `MOVW`, `MOVL` and `MOVQ` on amd64 and the corresponding instructions on other architectures). Operand names and offsets must match the Go declaration; taking an argument's address or accessing a subfield through `FP` is unsupported. Use the appropriate instruction width for each value, just as with ordinary assembly.

Callers still use the compiler's normal ABIInternal assignments. The compiler derives those assignments using its existing ABI analysis and passes package-local mappings to the final assembler in a work-directory sidecar; mappings are not exported. The assembler inserts parallel register copies on entry and before each `RET`, omitting identities and breaking cycles with an architecture-specific scratch register. With the example mapping and the current amd64 ABI, entry adaptation is just `MOVQ AX, DX`. Mapped inputs are live register aliases on body entry, not persistent stack slots: the body may clobber them and subsequent `FP` references observe those registers. The body must leave results in their mapped registers at each return.

The implementation supports scalar values assigned to exactly one integer register, including integers, booleans, pointers and pointer-sized values. It rejects methods, `init` functions, Go bodies, unnamed values, aggregates, floats, stack-assigned values and combinations with `//go:linkname` or `//go:cgo_unsafe_args`. Targets must be distinct within each side of `->` and use the canonical names below; aliases are not accepted. Scratch registers are reserved for adaptation. Stack, frame, link, goroutine, platform-reserved, other fixed ABI registers and floating-point/vector registers are unavailable as targets.

| Architecture | Allowed mapping targets | Adaptation scratch | Integer copy |
| --- | --- | --- | --- |
| amd64 | AX, BX, CX, DX, DI, SI, R8-R11, R13 | R12 | MOVQ |
| arm64 | R0-R15, R17, R19-R26 | R16 | MOVD |
| loong64 | R4-R19, R21, R23-R28 | R20 | MOVV |
| ppc64, ppc64le | R3-R12, R14-R29 | R31 | MOVD |
| riscv64 | X5-X26, X28-X30 | X31 | MOV |
| s390x | R2-R9 | R1 | MOVD |

Architectures without integer ABIInternal parameter registers in Go 1.27.1 (`386`, `arm`, `mips`, `mipsle`, `mips64`, `mips64le` and `wasm`) reject the directive.

Assembly must be zero-frame `NOSPLIT` leaf code, with no calls, tail calls, explicit stack operations or stack operands. Branches must use labels within the function. Normal assembly argument information and stack maps are retained through the existing metadata machinery. As with other ABIInternal assembly, authors must preserve the target ABI's fixed-register invariants, including the link register on link-register architectures and, on amd64, `R14` (the current goroutine) and zeroed `X15`; there is no ABI0 wrapper to restore them. Explicit `<ABIInternal>` selectors remain restricted to the usual privileged packages.

## Installation

**PACE 1.27.1 requires Go 1.27.1.** Each PACE release requires the exact matching standard Go release, not merely the same major/minor version. No second GOROOT or modified standard library is needed.

1. Install the matching official Go release normally.
2. Download the matching OS/architecture archive from [PACE releases](https://github.com/coalaura/pace/releases) and extract/copy its contents into that installation's GOROOT (`go env GOROOT`).
3. Run `pace version`.

Releases support Windows, Linux and macOS on amd64 and arm64. Windows archives are ZIP files; Linux and macOS archives are `.tar.gz` files. For example: `pace-1.27.1-windows-amd64.zip` or `pace-1.27.1-linux-arm64.tar.gz`.

The overlay contains exactly three executables, with `.exe` suffixes on Windows:

```text
$GOROOT/
  bin/
    go              (existing, unchanged)
    pace
    compilepe
    asmpe
  pace/
    README.md
    LICENSE
    PATENTS
    NOTICE
    licenses/       (vendored dependency notices, with source paths)
```

No stock Go files are overwritten. `pace` finds `compilepe` and `asmpe` beside its own executable and uses the matching GOROOT for all other tools, including the linker. Missing helpers or a mismatched GOROOT are errors. Running `go` remains unchanged.

`pace version` prints `go version go1.27.1 <os>/<arch> (pace)`, while `pace env GOVERSION` remains `go1.27.1`. PACE compiler and assembler tool IDs have a `pace` suffix, separating their build actions from stock Go in the normal shared build cache.

PACE stays on the local matching toolchain: automatic `GOTOOLCHAIN` switching and downloading are disabled. If a module requires a newer Go release, install the matching PACE and Go releases yourself. PACE does not collect or upload telemetry; `pace telemetry` is disabled and the stock Go telemetry configuration is left alone.

To uninstall, remove only `$GOROOT/bin/pace`, `$GOROOT/bin/compilepe`, `$GOROOT/bin/asmpe` and `$GOROOT/pace/` (use `.exe` filenames on Windows).

## Versioning and source builds

PACE release tags use `pace1.27.1`, `pace1.27.2`, `pace1.28.0` and so on, matching the corresponding Go release. PACE 1.27.1 is based on Go 1.27.1. Upstream-style `go1.27.1` tags identify upstream bases, not PACE releases.

PACE uses the ordinary [Go source-tree build process](https://go.dev/doc/install/source), with the matching official Go release as the bootstrap compiler. Source builds retain `bin/go` and the ordinary `compile`/`asm` tool names for bootstrap and development. The [release workflow](.github/workflows/release.yml) then uses that toolchain to cross-build `cmd/go`, `cmd/compile` and `cmd/asm` as `pace`, `compilepe` and `asmpe`. Distribution policy activates only under the release executable names.

## Upstream and license

PACE is based on the [Go source tree](https://go.googlesource.com/go). Upstream copyright and licensing are preserved; PACE modifications use the same BSD-style license. See [LICENSE](LICENSE), [PATENTS](PATENTS) and [NOTICE](NOTICE). Binary releases also include vendored dependency license and notice files under `pace/licenses/`; those files retain their original terms.
