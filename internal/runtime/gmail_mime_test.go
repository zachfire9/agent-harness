package runtime

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"unicode/utf8"
)

func decodeGmailBody(t *testing.T, encoded string) (*mail.Message, []string) {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(string(raw), "\r\n", ""), "\n") {
		t.Fatal("MIME has bare LF")
	}
	message, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	media, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	if media != "multipart/alternative" || message.Header.Get("MIME-Version") != "1.0" {
		t.Fatalf("not MIME multipart/alternative: %v", message.Header)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	var bodies []string
	for _, want := range []string{"text/plain", "text/html"} {
		part, err := reader.NextRawPart()
		if err != nil {
			t.Fatal(err)
		}
		media, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}
		if media != want || strings.ToUpper(params["charset"]) != "UTF-8" {
			t.Fatalf("bad part type: %v", part.Header)
		}
		if part.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
			t.Fatalf("missing UTF8 transfer encoding: %v", part.Header)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(part))
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, strings.ReplaceAll(string(body), "\r\n", "\n"))
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("expected exactly two parts: %v", err)
	}
	return message, bodies
}

func TestGmailMultipartNormalizesTextForUTF8MIME(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"Café\r\n茶\rNext\nLast", "Café\n茶\nNext\nLast"},
		{"Café " + string([]byte{0xe8, 0x8c}), "Café \uFFFD"},
		{strings.Repeat("茶 & ", 100), strings.Repeat("茶 & ", 100)},
	} {
		_, parts := decodeGmailBody(t, encodeGmailRawMessage(GmailNotifierConfig{BodyFormat: "multipart"}, Message{Body: tc.input}))
		if parts[0] != tc.want {
			t.Fatalf("plain normalization: got %q, want %q", parts[0], tc.want)
		}
		if !utf8.ValidString(parts[0]) || !utf8.ValidString(parts[1]) {
			t.Fatal("invalid UTF-8 in decoded MIME part")
		}
	}
}

func TestGmailMultipartMIME(t *testing.T) {
	body := "# Café & 茶\n\nFirst line\nSecond <line>\n\n- naïve\n* 日本語\n\n## Next\n### Third"
	config := GmailNotifierConfig{From: "agent@example.com", To: []string{"reader@example.com"}, SubjectPrefix: "Résumé", BodyFormat: "multipart"}
	message, bodies := decodeGmailBody(t, encodeGmailRawMessage(config, Message{Job: "日報", Body: body}))
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Résumé 日報" {
		t.Fatalf("bad subject %q", subject)
	}
	if bodies[0] != body {
		t.Fatalf("plain fallback changed: %q", bodies[0])
	}
	want := "<!doctype html><html><body><h1>Café &amp; 茶</h1><p>First line<br>Second &lt;line&gt;</p><ul><li>naïve</li><li>日本語</li></ul><h2>Next</h2><h3>Third</h3></body></html>"
	if bodies[1] != want {
		t.Fatalf("HTML got %q, want %q", bodies[1], want)
	}
}

func TestGmailMultipartEscapesHostileText(t *testing.T) {
	for _, body := range []string{
		`<script>alert("x")</script><img src=x onerror=alert(1)>`,
		`# <a href="javascript:alert(1)">link</a>`,
		"- <svg onload='alert(1)'>&\n- [link](javascript:alert(1))\n- ![image](https://example.com/a.png)",
		"Ordinary text\nStill readable\n\nNew paragraph",
		"#### Unsupported heading\n**literal bold**\n\n", "", "# \n- \n* ",
	} {
		t.Run(body, func(t *testing.T) {
			_, parts := decodeGmailBody(t, encodeGmailRawMessage(GmailNotifierConfig{BodyFormat: "multipart"}, Message{Body: body}))
			for _, forbidden := range []string{"<script", "<img", "<svg", "<a ", "<strong", "<h4"} {
				if strings.Contains(parts[1], forbidden) {
					t.Fatalf("unsafe/unsupported HTML %q", parts[1])
				}
			}
			if strings.Contains(body, "<script") && !strings.Contains(parts[1], "&lt;script&gt;") {
				t.Fatal("hostile text not preserved escaped")
			}
			if body == "Ordinary text\nStill readable\n\nNew paragraph" && parts[1] != "<!doctype html><html><body><p>Ordinary text<br>Still readable</p><p>New paragraph</p></body></html>" {
				t.Fatalf("plain text unreadable: %q", parts[1])
			}
		})
	}
}

func TestGmailPlainBackwardsCompatibility(t *testing.T) {
	for _, format := range []string{"", "plain"} {
		raw := encodeGmailRawMessage(GmailNotifierConfig{From: "agent@example.com", To: []string{"reader@example.com"}, SubjectPrefix: "[Note]", BodyFormat: format}, Message{Job: "daily", Body: "Café\n# Literal heading\n<literal>"})
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		want := "From: <agent@example.com>\r\nTo: reader@example.com\r\nSubject: [Note] daily\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nCafé\n# Literal heading\n<literal>\r\n"
		if string(decoded) != want {
			t.Fatalf("plain behavior changed: %q", decoded)
		}
	}
}
