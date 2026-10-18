//go:build windows

package main

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

// Windows DPAPI binds encrypted data to the current Windows user account.
// The cryptographic keys are managed by Windows, never embedded in the app.
var (
	crypt32            = syscall.NewLazyDLL("crypt32.dll")
	cryptProtectData   = crypt32.NewProc("CryptProtectData")
	cryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	localFree          = kernel32.NewProc("LocalFree")
)

const cryptProtectUIForbidden = 0x1

type dataBlob struct {
	len  uint32
	data *byte
}

func cacheAvailable() bool { return true }

func dpapiProtect(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("нельзя зашифровать пустой буфер")
	}
	in := dataBlob{len: uint32(len(plaintext)), data: &plaintext[0]}
	var out dataBlob
	ok, _, callErr := cryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt structure
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(plaintext)
	runtime.KeepAlive(in)
	if ok == 0 {
		return nil, callErr
	}
	defer freeBlob(out.data)
	return append([]byte(nil), unsafe.Slice(out.data, out.len)...), nil
}

func dpapiUnprotect(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, errors.New("пустой файл кеша")
	}
	in := dataBlob{len: uint32(len(ciphertext)), data: &ciphertext[0]}
	var out dataBlob
	ok, _, callErr := cryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, // description
		0, // optional entropy
		0, // reserved
		0, // prompt structure
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(ciphertext)
	runtime.KeepAlive(in)
	if ok == 0 {
		return nil, callErr
	}
	defer freeBlob(out.data)
	return append([]byte(nil), unsafe.Slice(out.data, out.len)...), nil
}

func freeBlob(p *byte) {
	if p != nil {
		_, _, _ = localFree.Call(uintptr(unsafe.Pointer(p)))
	}
}
