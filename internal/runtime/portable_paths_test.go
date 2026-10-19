package runtime

import "testing"

func TestPortableRelativePaths(t *testing.T) {
	for _, path := range []string{`C:\outside`, `C:outside`, `\\server\share`, `\outside`, `/outside`, `work\..\..\outside`, `work/item:stream`, `work/NUL`, `work/con.txt`, `work/item.`, `work/item `} {
		if err := validateInstanceRelativePath(path); err == nil {
			t.Errorf("accepted nonportable/escaping path %q", path)
		}
	}
	for _, path := range []string{"work/items.jsonl", "config/secrets/google-token.json"} {
		if err := validateInstanceRelativePath(path); err != nil {
			t.Fatal(err)
		}
	}
}
