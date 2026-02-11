package wepoll

//go:generate go run golang.org/x/sys/windows/mkwinsyscall -output zsyscall_windows.go syscall_windows.go

//sys NtCreateFile(handle *windows.Handle, access uint32, oa *ObjectAttributes, iosb *windows.IO_STATUS_BLOCK, allocationSize *int64, attributes uint32, share uint32, disposition uint32, options uint32, eaBuffer uintptr, eaLength uint32) (ntstatus error) = ntdll.NtCreateFile
//sys NtDeviceIoControlFile(handle windows.Handle, event windows.Handle, apcRoutine uintptr, apcContext uintptr, ioStatusBlock *windows.IO_STATUS_BLOCK, ioControlCode uint32, inputBuffer unsafe.Pointer, inputBufferLength uint32, outputBuffer unsafe.Pointer, outputBufferLength uint32) (ntstatus error) = ntdll.NtDeviceIoControlFile
//sys NtCancelIoFileEx(handle windows.Handle, ioRequestToCancel *windows.IO_STATUS_BLOCK, ioStatusBlock *windows.IO_STATUS_BLOCK) (ntstatus error) = ntdll.NtCancelIoFileEx
//sys GetQueuedCompletionStatusEx(cphandle windows.Handle, entries *OverlappedEntry, count uint32, numRemoved *uint32, timeout uint32, alertable bool) (err error) = kernel32.GetQueuedCompletionStatusEx
