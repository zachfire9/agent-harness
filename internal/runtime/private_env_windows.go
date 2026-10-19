package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ACL validation uses a fixed script and a separately passed path, never file
// contents. No profile scripts, shell expansion of paths, or credential output.
func checkPrivateEnvironmentFile(path string, _ os.FileInfo) error {
	script := `$ErrorActionPreference = 'Stop'
$acl = Get-Acl -LiteralPath $env:AGENT_HARNESS_ACL_PATH
$descriptor = [System.Security.AccessControl.RawSecurityDescriptor]::new($acl.GetSecurityDescriptorBinaryForm(), 0)
# A null/absent DACL grants unrestricted access. A present empty DACL denies
# access instead; do not confuse its lack of ACEs with a null DACL.
if (($descriptor.ControlFlags -band [System.Security.AccessControl.ControlFlags]::DiscretionaryAclPresent) -eq 0 -or $null -eq $descriptor.DiscretionaryAcl) { exit 1 }
$me = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$allowed = @($me, 'S-1-5-18', 'S-1-5-32-544')
foreach ($rule in $acl.Access) {
 if ($rule.AccessControlType -eq 'Allow') {
  $sid = $rule.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value
  if ($allowed -notcontains $sid) { exit 1 }
 }
}
$owner = $acl.GetOwner([System.Security.Principal.SecurityIdentifier]).Value
if ($allowed -notcontains $owner) { exit 1 }
`
	// Use the OS installation, not PATH (which can be caller-controlled).
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(exe, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "AGENT_HARNESS_ACL_PATH="+path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("private environment ACL must allow only the current user, SYSTEM, and Administrators; see Windows runtime docs")
	}
	return nil
}
