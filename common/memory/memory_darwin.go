package memory

// #include <mach/mach.h>
// #include <stddef.h>
// #include <dlfcn.h>
//
// typedef size_t (*proc_available_memory_func)(void);
// static int resolved = 0;
// static proc_available_memory_func fn = NULL;
//
// static void resolve_available_memory() {
//     if (!resolved) {
//         fn = (proc_available_memory_func)dlsym(RTLD_DEFAULT, "os_proc_available_memory");
//         resolved = 1;
//     }
// }
//
// static size_t get_available_memory(int *supported) {
//     resolve_available_memory();
//     if (fn) {
//         *supported = 1;
//         return fn();
//     }
//     *supported = 0;
//     return 0;
// }
//
// static int is_available_memory_supported() {
//     resolve_available_memory();
//     return fn != NULL;
// }
import "C"
import "unsafe"

func totalNative() uint64 {
	var vmInfo C.task_vm_info_data_t
	var count C.mach_msg_type_number_t = C.TASK_VM_INFO_COUNT
	if C.task_info(C.vm_map_t(C.mach_task_self_), C.TASK_VM_INFO, (*C.integer_t)(unsafe.Pointer(&vmInfo)), &count) == C.KERN_SUCCESS {
		return uint64(vmInfo.phys_footprint)
	}
	return 0
}

func totalAvailable() bool {
	return true
}

func availableNative() uint64 {
	var supported C.int
	result := C.get_available_memory(&supported)
	if supported == 0 {
		return 0
	}
	return uint64(result)
}

func availableAvailable() bool {
	return C.is_available_memory_supported() != 0
}
