# The Go Programming Language

Go is an open source programming language that makes it easy to build simple,
reliable, and efficient software.

![Gopher image](https://golang.org/doc/gopher/fiveyears.jpg)
*Gopher image by [Renee French][rf], licensed under [Creative Commons 4.0 Attribution license][cc4-by].*

Our canonical Git repository is located at https://go.googlesource.com/go.
There is a mirror of the repository at https://github.com/golang/go.

Unless otherwise noted, the Go source files are distributed under the
BSD-style license found in the LICENSE file.

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

### Download and Install

#### Binary Distributions

Official binary distributions are available at https://go.dev/dl/.

After downloading a binary release, visit https://go.dev/doc/install
for installation instructions.

#### Install From Source

If a binary distribution is not available for your combination of
operating system and architecture, visit
https://go.dev/doc/install/source
for source installation instructions.

### Contributing

Go is the work of thousands of contributors. We appreciate your help!

To contribute, please read the contribution guidelines at https://go.dev/doc/contribute.

Note that the Go project uses the issue tracker for bug reports and
proposals only. See https://go.dev/wiki/Questions for a list of
places to ask questions about the Go language.

[rf]: https://reneefrench.blogspot.com/
[cc4-by]: https://creativecommons.org/licenses/by/4.0/
