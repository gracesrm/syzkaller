// Copyright 2017 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// +build

#include <stdio.h>

#define PRINT(x)                                   \
	extern const unsigned char x[], x##_end[]; \
	print(#x, x, x##_end);

void print(const char* name, const unsigned char* start, const unsigned char* end)
{
	printf("const char %s[] = \"", name);
	for (const unsigned char* p = start; p < end; p++)
		printf("\\x%02x", *p);
	printf("\";\n");
}

int main()
{
	printf("// AUTOGENERATED FILE\n");
	PRINT(kvm_asm16_cpl3);
	PRINT(kvm_asm32_paged);
	PRINT(kvm_asm32_vm86);
	PRINT(kvm_asm32_paged_vm86);
	PRINT(kvm_asm64_vm86);
	PRINT(kvm_asm64_enable_long);
	PRINT(kvm_asm64_init_vm);
	PRINT(kvm_asm64_vm_exit);
	PRINT(kvm_asm64_cpl3);
	return 0;
}
