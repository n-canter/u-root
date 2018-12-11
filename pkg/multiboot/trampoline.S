/*
 * purgatory:  setup code
 *
 * Copyright (C) 2003,2004  Eric Biederman (ebiederm@xmission.com)
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation (version 2 of the License).
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 675 Mass Ave, Cambridge, MA 02139, USA.
    */

.text
	.equ	MSR_K6_EFER,   0xC0000080
	.equ	EFER_LME,      0x00000100
	.equ	X86_CR4_PAE,   0x00000020
	.equ	CR0_PG,        0x80000000
_start:
    .code64
	/* Setup the 4G offset of entry32 lm_exit code segment */
    	movq	$0x00CF9A000000ffff, %rax

	leaq	_start(%rip), %rbx	/* Low 24 bits */
	andq	$0xffffff, %rbx
	shlq	$16, %rbx
        orq	%rbx, %rax

        leaq	_start(%rip), %rbx	/* High 8 bits */
	movq	$0xff000000, %rdx
	andq	%rdx, %rbx
	shlq	$32, %rbx
	orq	%rbx, %rax

	movq	%rax, (gdt + 0x20)(%rip)

    	/* Setup a gdt that is generally usefully */
	leaq	gdt(%rip), %rcx
    	movq 	%rcx, (gdt + 0x2)(%rip)
    
	lgdt	gdt(%rip)

	/* Setup the far pointer to the entry point */
	movl	eip(%rip), %eax
	movl	%eax, entry32_addr(%rip)

	/* Switch to 32bit compatiblity mode */
        ljmp	*lm_exit_addr(%rip)
    
lm_exit:
    .code32
	/* Disable paging */
	movl	%cr0, %eax
	andl	$~CR0_PG, %eax
        movl	%eax, %cr0

	/* Disable long mode */
	movl	$MSR_K6_EFER, %ecx
	rdmsr
	andl	$~EFER_LME, %eax
	wrmsr

	/* Disable PAE */
	xorl	%eax, %eax
	movl	%eax, %cr4

	/* load the data segments */
	movl	$0x18, %eax	/* data segment */
	movl	%eax, %ds
	movl	%eax, %es
	movl	%eax, %ss
	movl	%eax, %fs
	movl	%eax, %gs

	/* Load the registers */
    	movl	%cs:eax - _start, %eax
        movl	%cs:ebx - _start, %ebx  
        /* Jump to the loaded image */
        ljmp	*%cs:entry32_addr - _start
    
gdt:	/* 0x00 unusable segment 
	 * 0x08 unused
	 * so use them as the gdt ptr
	 */
	.word	gdt_end - gdt - 1
	.quad	0
	.word	0, 0, 0
			
	/* Documented linux kernel segments */
	/* 0x10 4GB flat code segment */
	.word	0xFFFF, 0x0000, 0x9A00, 0x00CF
	/* 0x18 4GB flat data segment */
	.word	0xFFFF, 0x0000, 0x9200, 0x00CF

	/* 0x20 4GB flat code segment base at entry32 */
	.word	0xFFFF, 0x0000, 0x9A00, 0x0CF
	/* 0x28 dummy */
	
	/* Segments used by the 2.5.x kernel */
	/* 0x60 4GB flat code segment */
	.word	0xFFFF, 0x0000, 0x9A00, 0x00CF
	/* 0x68 4GB flat data segment */
	.word	0xFFFF, 0x0000, 0x9200, 0x00CF
gdt_end:

lm_exit_addr:
	.long lm_exit - _start
	.long 0x20

entry32_addr:
	.long 0x0
	.long 0x10

entry32_regs:
	eax:	.long 0x2BADB002
	.ascii "u-root-ebx-long"
	ebx:	.long 0x00000000
	.ascii "u-root-ep-quad"
	eip:	.quad 0x0000000000000000


