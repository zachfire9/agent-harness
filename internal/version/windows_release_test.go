package version

import (
	"regexp"
	"strings"
	"testing"
)

// These source contracts run on Linux too; native ACL/PowerShell behavior still
// requires the Windows CI runner.
func TestWindowsACLRejectsMissingDACLContract(t *testing.T) {
	source := readRepoFile(t, "internal", "runtime", "private_env_windows.go")
	for _, want := range []string{"RawSecurityDescriptor", "$acl.GetSecurityDescriptorBinaryForm()", "DiscretionaryAclPresent", "$null -eq $descriptor.DiscretionaryAcl"} {
		if !strings.Contains(source, want) {
			t.Errorf("null/absent DACL guard missing %q", want)
		}
	}
	if strings.Contains(source, "DiscretionaryAcl.Count -eq 0") {
		t.Fatal("must distinguish an empty DACL (deny all) from a null DACL")
	}
}

func TestWindowsStatusPollRetriesReadAndParseContract(t *testing.T) {
	smoke := readRepoFile(t, "scripts", "test-windows-runtime.ps1")
	guard := regexp.MustCompile(`(?s)try\s*\{\s*\$script:live = Get-Content[^\n]*-ErrorAction Stop[^\n]*\| ConvertFrom-Json -ErrorAction Stop\s*\}\s*catch\s*\{\s*return \$false\s*\}`)
	if !guard.MatchString(smoke) {
		t.Fatal("status file read/parse must return false on a transient first-write failure")
	}
	for _, want := range []string{"UtcNow.AddSeconds(30)", "while ([DateTime]::UtcNow -lt $deadline)", "if (& $Condition) { return }", "return $script:live.status -eq 'running' -and $script:live.last_heartbeat_at"} {
		if !strings.Contains(smoke, want) {
			t.Errorf("bounded polling/assertion contract missing %q", want)
		}
	}
}

func TestWindowsReleaseAndNativeCI(t *testing.T) {
	script := readRepoFile(t, "scripts", "build-release.sh")
	for _, want := range []string{`"windows amd64"`, `binary_name="agent-harness.exe"`, `.zip`, `zipfile`, `*.tar.gz`, `*.zip`, `windows-runtime.md`} {
		if !strings.Contains(script, want) {
			t.Errorf("build script missing %q", want)
		}
	}
	workflow := readRepoFile(t, ".github", "workflows", "release.yml")
	if !strings.Contains(workflow, `dist/agent-harness_${VERSION}_windows_amd64.zip`) {
		t.Error("release omits Windows zip")
	}
	for _, want := range []string{"uses: ./.github/workflows/test.yml", "needs: verify"} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release is not gated by native verification: missing %q", want)
		}
	}
	ci := readRepoFile(t, ".github", "workflows", "test.yml")
	for _, want := range []string{"windows-latest", "ubuntu-latest", "go test ./...", "go vet ./...", "scripts/test-windows-runtime.ps1"} {
		if !strings.Contains(ci, want) {
			t.Errorf("native CI missing %q", want)
		}
	}
	smoke := readRepoFile(t, "scripts", "test-windows-runtime.ps1")
	for _, want := range []string{"Start-ScheduledTask", "Stop-ScheduledTask", "Unregister-ScheduledTask", "finally", "last_heartbeat_at", "--test", "--json", "jobs", "Get-Acl"} {
		if !strings.Contains(smoke, want) {
			t.Errorf("native smoke missing %q", want)
		}
	}
	for _, want := range []string{"NATIVE TASK LIFECYCLE UNAVAILABLE", "throw", "SessionId", "Set-Acl", "ConvertFrom-Json"} {
		if !strings.Contains(smoke, want) {
			t.Errorf("native evidence/ACL contract missing %q", want)
		}
	}
	docs := readRepoFile(t, "docs", "windows-runtime.md")
	for _, want := range []string{"Interactive", "Limited", "Get-FileHash", "Get-Acl", "snapshot", "after logout"} {
		if !strings.Contains(docs, want) {
			t.Errorf("Windows guide missing %q", want)
		}
	}
	source := readRepoFile(t, "internal", "runtime", "tzdata_windows.go")
	if !strings.Contains(source, `"time/tzdata"`) {
		t.Error("Windows binary lacks IANA timezone database")
	}
}
