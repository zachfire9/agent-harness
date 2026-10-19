package cli

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestPowerShellScriptEncodingPreservesUnicode(t *testing.T) {
	raw := "$p='C:\\Users\\test ü\\it''s';"
	encoded := encodePowerShell(raw)
	b, e := base64.StdEncoding.DecodeString(encoded)
	if e != nil {
		t.Fatal(e)
	}
	v := make([]uint16, len(b)/2)
	for i := range v {
		v[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	if string(utf16.Decode(v)) != raw || strings.Contains(encoded, raw) {
		t.Fatal("script corrupted")
	}
}
