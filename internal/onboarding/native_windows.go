//go:build windows

package onboarding

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var getMode = kernel.NewProc("GetConsoleMode")
var setMode = kernel.NewProc("SetConsoleMode")

func terminal(f *os.File) bool {
	var m uint32
	r, _, _ := getMode.Call(f.Fd(), uintptr(unsafe.Pointer(&m)))
	return r != 0
}
func disableEcho(f *os.File) (func() error, error) {
	var m uint32
	r, _, _ := getMode.Call(f.Fd(), uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return nil, errors.New("console required")
	}
	r, _, _ = setMode.Call(f.Fd(), uintptr(m&^4))
	if r == 0 {
		return nil, errors.New("cannot disable echo")
	}
	return func() error {
		r, _, _ := setMode.Call(f.Fd(), uintptr(m))
		if r == 0 {
			return errors.New("cannot restore console mode")
		}
		return nil
	}, nil
}

var advapi = syscall.NewLazyDLL("advapi32.dll")
var credRead = advapi.NewProc("CredReadW")
var credWrite = advapi.NewProc("CredWriteW")
var credDelete = advapi.NewProc("CredDeleteW")
var credFree = advapi.NewProc("CredFree")

type credential struct {
	Flags, Type             uint32
	Target, Comment         *uint16
	LastWritten             syscall.Filetime
	BlobSize                uint32
	Blob                    *byte
	Persist, AttributeCount uint32
	Attributes              uintptr
	Alias, User             *uint16
}
type windowsStore struct{}

func NativeStore() CredentialStore { return windowsStore{} }
func validTarget(s string) bool {
	return strings.HasPrefix(s, "agent-harness/interactive/") && !strings.ContainsAny(s, "\r\n\x00")
}
func (windowsStore) Get(target string) (string, error) {
	if !validTarget(target) {
		return "", errors.New("invalid credential reference")
	}
	p, e := syscall.UTF16PtrFromString(target)
	if e != nil {
		return "", errors.New("invalid credential reference")
	}
	var c *credential
	r, _, _ := credRead.Call(uintptr(unsafe.Pointer(p)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		return "", errors.New("Credential Manager read failed")
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c.BlobSize == 0 || c.BlobSize > 5120 || c.BlobSize%2 != 0 {
		return "", errors.New("invalid credential data")
	}
	return string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(c.Blob)), int(c.BlobSize)/2))), nil
}
func (windowsStore) Put(target, key string) error {
	if !validTarget(target) {
		return errors.New("invalid credential reference")
	}
	p, e := syscall.UTF16PtrFromString(target)
	if e != nil {
		return errors.New("invalid credential reference")
	}
	b := utf16.Encode([]rune(key))
	defer func() {
		for i := range b {
			b[i] = 0
		}
	}()
	if len(b) == 0 || len(b) > 2560 {
		return errors.New("credential is empty or too long")
	}
	user, _ := syscall.UTF16PtrFromString("agent-harness")
	c := credential{Type: 1, Target: p, BlobSize: uint32(len(b) * 2), Blob: (*byte)(unsafe.Pointer(&b[0])), Persist: 2, User: user}
	r, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	if r == 0 {
		return errors.New("Credential Manager write failed")
	}
	return nil
}
func (windowsStore) Delete(target string) error {
	if !validTarget(target) {
		return errors.New("invalid credential reference")
	}
	p, e := syscall.UTF16PtrFromString(target)
	if e != nil {
		return errors.New("invalid credential reference")
	}
	r, _, errno := credDelete.Call(uintptr(unsafe.Pointer(p)), 1, 0)
	if r == 0 && errno != syscall.Errno(1168) {
		return errors.New("Credential Manager delete failed")
	}
	return nil
}
