// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"
#include "funcdata.h"

TEXT ·Optimized(SB), NOSPLIT, $0-32
	MOVQ addr+0(FP), DX
	MOVQ new+8(FP), BX
	MOVB width+16(FP), CX
	MOVB signed+17(FP), DI
	MOVQ (DX), AX
	ADDQ BX, AX
	MOVBQZX CX, CX
	ADDQ CX, AX
	TESTB DI, DI
	JE optimizedDone
	INCQ AX
optimizedDone:
	MOVQ AX, old+24(FP)
	RET

TEXT ·Different(SB), NOSPLIT, $0-32
	MOVQ addr+0(FP), BX
	MOVQ new+8(FP), DX
	MOVB width+16(FP), R8
	MOVB signed+17(FP), R9
	MOVQ (BX), R10
	ADDQ DX, R10
	MOVBQZX R8, R8
	ADDQ R8, R10
	TESTB R9, R9
	JE differentDone
	INCQ R10
differentDone:
	MOVQ R10, old+24(FP)
	RET

TEXT ·Cycle(SB), NOSPLIT, $0-48
	MOVQ a+0(FP), BX
	MOVQ b+8(FP), CX
	MOVQ c+16(FP), AX
	TESTQ BX, BX
	JE cycleZero
	MOVQ BX, x+24(FP)
	MOVQ CX, y+32(FP)
	MOVQ AX, z+40(FP)
	RET
cycleZero:
	MOVQ BX, x+24(FP)
	MOVQ CX, y+32(FP)
	MOVQ AX, z+40(FP)
	JMP cycleReturn
cycleReturn:
	RET

TEXT ·Identity(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ AX, result+8(FP)
	RET

TEXT ·Pointer(SB), NOSPLIT, $0-16
	GO_ARGS
	NO_LOCAL_POINTERS
	MOVQ a+0(FP), DX
	MOVQ DX, result+8(FP)
	RET

TEXT ·Ordinary(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ AX, result+8(FP)
	RET

TEXT ·Narrow(SB), NOSPLIT, $0-16
	MOVW a+0(FP), AX
	MOVL b+4(FP), BX
	MOVW AX, x+8(FP)
	MOVL BX, y+12(FP)
	RET

TEXT ·Referenced(SB), NOSPLIT, $0-16
	MOVQ a+0(FP), AX
	MOVQ AX, result+8(FP)
	RET

TEXT ·Legacy(SB), NOSPLIT, $16-16
	NO_LOCAL_POINTERS
	MOVQ a+0(FP), AX
	MOVQ AX, 0(SP)
	CALL ·Referenced(SB)
	MOVQ 8(SP), AX
	MOVQ AX, result+8(FP)
	RET
