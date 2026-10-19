package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestPrivateEnvironmentRejectsNullDACL(t *testing.T) {
	paths := PathsForHome(t.TempDir())
	path := filepath.Join(paths.ConfigDir, "secrets", "env")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	const key = "HARNESS_NULL_DACL_API_KEY"
	if err := os.WriteFile(path, []byte(key+"=fake-null-dacl-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	setEnvironmentFixturePrivacy(t, path, true) // Explicitly allowed current-user owner.
	// Restore before TempDir cleanup, including on assertion/setup failures.
	t.Cleanup(func() { setEnvironmentFixturePrivacy(t, path, true) })
	t.Setenv(key, "unchanged")
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	setSecurity := syscall.NewLazyDLL("advapi32.dll").NewProc("SetNamedSecurityInfoW")
	// SE_FILE_OBJECT, DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION.
	// A null DACL grants everyone access, unlike a present zero-ACE (empty) DACL.
	result, _, _ := setSecurity.Call(uintptr(unsafe.Pointer(name)), 1, 0x80000004, 0, 0, 0, 0)
	if result != 0 {
		t.Fatalf("set fake fixture null DACL: %v", syscall.Errno(result))
	}
	err = LoadPrivateEnvironment(paths)
	if err == nil || !strings.Contains(err.Error(), "private environment ACL") {
		t.Fatalf("expected null DACL rejection, got %v", err)
	}
	if os.Getenv(key) != "unchanged" {
		t.Fatal("rejected null DACL fixture mutated environment")
	}
}

func setEnvironmentFixturePrivacy(t *testing.T, path string, private bool) {
	t.Helper()
	// Fake credentials only. Replace the DACL rather than trusting inherited ACLs.
	script := `$ErrorActionPreference = 'Stop'
$sid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$acl = Get-Acl -LiteralPath $env:AGENT_HARNESS_ACL_PATH
$acl.SetSecurityDescriptorSddlForm('O:' + $sid + 'D:P(A;;FA;;;' + $sid + ')')
if ($env:AGENT_HARNESS_TEST_BROAD -eq '1') {
 $rule = [System.Security.AccessControl.FileSystemAccessRule]::new([System.Security.Principal.SecurityIdentifier]::new('S-1-1-0'), 'Read', 'Allow')
 $acl.AddAccessRule($rule)
}
Set-Acl -LiteralPath $env:AGENT_HARNESS_ACL_PATH -AclObject $acl
`
	broad := "0"
	if !private {
		broad = "1"
	}
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(exe, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "AGENT_HARNESS_ACL_PATH="+path, "AGENT_HARNESS_TEST_BROAD="+broad)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set fake fixture ACL: %v: %s", err, out)
	}
}
