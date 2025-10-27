package smtp

import (
	"testing"
)

func TestParseMIMEMessage_SimpleText(t *testing.T) {
	raw := []byte("Subject: Test\nFrom: sender@example.com\nContent-Type: text/plain; charset=utf-8\n\nHello, World!")

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.Subject != "Test" {
		t.Errorf("expected subject 'Test', got '%s'", email.Subject)
	}
	if email.Text != "Hello, World!" {
		t.Errorf("expected text 'Hello, World!', got '%s'", email.Text)
	}
	if email.From != "sender@example.com" {
		t.Errorf("expected from 'sender@example.com', got '%s'", email.From)
	}
}

func TestParseMIMEMessage_SimpleHTML(t *testing.T) {
	raw := []byte("Subject: Test\nFrom: sender@example.com\nContent-Type: text/html; charset=utf-8\n\n<b>Hello</b>")

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.HTML != "<b>Hello</b>" {
		t.Errorf("expected html '<b>Hello</b>', got '%s'", email.HTML)
	}
}

func TestParseMIMEMessage_MultipartAlternative(t *testing.T) {
	boundary := "boundary12345"
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: multipart/alternative; boundary=` + boundary + `

--` + boundary + `
Content-Type: text/plain

Plain text version
--` + boundary + `
Content-Type: text/html

<b>HTML version</b>
--` + boundary + `--
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.Text != "Plain text version" {
		t.Errorf("expected text 'Plain text version', got '%s'", email.Text)
	}
	if email.HTML != "<b>HTML version</b>" {
		t.Errorf("expected html '<b>HTML version</b>', got '%s'", email.HTML)
	}
}

func TestParseMIMEMessage_WithAttachment(t *testing.T) {
	boundary := "boundary12345"
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: multipart/mixed; boundary=` + boundary + `

--` + boundary + `
Content-Type: text/plain

Body text
--` + boundary + `
Content-Disposition: attachment; filename="test.txt"

Attachment content
--` + boundary + `--
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if len(email.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(email.Attachments))
	}
	if email.Attachments[0].Filename != "test.txt" {
		t.Errorf("expected filename 'test.txt', got '%s'", email.Attachments[0].Filename)
	}
	if string(email.Attachments[0].Content) != "Attachment content" {
		t.Errorf("expected content 'Attachment content', got '%s'", string(email.Attachments[0].Content))
	}
}

func TestParseMIMEMessage_Base64Encoding(t *testing.T) {
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: base64

SGVsbG8gV29ybGQh
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	expected := "Hello World!"
	if email.Text != expected {
		t.Errorf("expected text '%s', got '%s'", expected, email.Text)
	}
}

func TestParseMIMEMessage_QuotedPrintableEncoding(t *testing.T) {
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

Hello=20World!
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	expected := "Hello World!\n"
	if email.Text != expected {
		t.Errorf("expected text '%s', got '%s'", expected, email.Text)
	}
}

func TestParseMIMEMessage_WithCCAndBCC(t *testing.T) {
	raw := []byte(`Subject: Test
From: sender@example.com
Cc: cc@example.com
Bcc: bcc@example.com
Reply-To: reply@example.com

Body text
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if len(email.Cc) != 1 || email.Cc[0] != "cc@example.com" {
		t.Errorf("expected CC 'cc@example.com', got %v", email.Cc)
	}
	if len(email.Bcc) != 1 || email.Bcc[0] != "bcc@example.com" {
		t.Errorf("expected BCC 'bcc@example.com', got %v", email.Bcc)
	}
	if email.ReplyTo != "reply@example.com" {
		t.Errorf("expected Reply-To 'reply@example.com', got '%s'", email.ReplyTo)
	}
}

func TestParseMIMEMessage_MultipleRecipients(t *testing.T) {
	raw := []byte("Subject: Test\nFrom: sender@example.com\n\nBody")

	email := ParseMIMEMessage("sender@example.com", []string{"r1@example.com", "r2@example.com"}, raw)

	if len(email.To) != 2 {
		t.Errorf("expected 2 recipients, got %d", len(email.To))
	}
}

func TestParseMIMEMessage_MultipleAttachments(t *testing.T) {
	boundary := "boundary12345"
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: multipart/mixed; boundary=` + boundary + `

--` + boundary + `
Content-Type: text/plain

Body text
--` + boundary + `
Content-Disposition: attachment; filename="file1.txt"

Content 1
--` + boundary + `
Content-Disposition: attachment; filename="file2.txt"

Content 2
--` + boundary + `--
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if len(email.Attachments) != 2 {
		t.Errorf("expected 2 attachments, got %d", len(email.Attachments))
	}
	if email.Attachments[0].Filename != "file1.txt" {
		t.Errorf("expected first filename 'file1.txt', got '%s'", email.Attachments[0].Filename)
	}
	if email.Attachments[1].Filename != "file2.txt" {
		t.Errorf("expected second filename 'file2.txt', got '%s'", email.Attachments[1].Filename)
	}
}

func TestParseMIMEMessage_CustomHeaders(t *testing.T) {
	raw := []byte(`Subject: Test
From: sender@example.com
X-Custom-Header: Custom Value
X-Another-Header: Another Value

Body text
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.Headers["X-Custom-Header"] != "Custom Value" {
		t.Errorf("expected X-Custom-Header 'Custom Value', got '%s'", email.Headers["X-Custom-Header"])
	}
	if email.Headers["X-Another-Header"] != "Another Value" {
		t.Errorf("expected X-Another-Header 'Another Value', got '%s'", email.Headers["X-Another-Header"])
	}
}

func TestParseMIMEMessage_NestedMultipart(t *testing.T) {
	outerBoundary := "outer12345"
	innerBoundary := "inner12345"
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: multipart/mixed; boundary=` + outerBoundary + `

--` + outerBoundary + `
Content-Type: multipart/alternative; boundary=` + innerBoundary + `

--` + innerBoundary + `
Content-Type: text/plain

Plain
--` + innerBoundary + `
Content-Type: text/html

<b>HTML</b>
--` + innerBoundary + `--
--` + outerBoundary + `
Content-Disposition: attachment; filename="file.txt"

Attachment content
--` + outerBoundary + `--
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.Text != "Plain" {
		t.Errorf("expected text 'Plain', got '%s'", email.Text)
	}
	if email.HTML != "<b>HTML</b>" {
		t.Errorf("expected html '<b>HTML</b>', got '%s'", email.HTML)
	}
	if len(email.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(email.Attachments))
	}
}

func TestParseMIMEMessage_InlineAttachment(t *testing.T) {
	boundary := "boundary12345"
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: multipart/related; boundary=` + boundary + `

--` + boundary + `
Content-Type: text/html

<img src="cid:image.png">
--` + boundary + `
Content-Type: image/png
Content-Disposition: inline; filename="image.png"

PNG content
--` + boundary + `--
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.HTML != `<img src="cid:image.png">` {
		t.Errorf("expected HTML with inline reference, got '%s'", email.HTML)
	}
	if len(email.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(email.Attachments))
	}
}

func TestParseMIMEMessage_EmptyMessage(t *testing.T) {
	raw := []byte("Subject: Test\nFrom: sender@example.com\n\n")

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if email.Text != "" {
		t.Errorf("expected empty text, got '%s'", email.Text)
	}
	if email.HTML != "" {
		t.Errorf("expected empty html, got '%s'", email.HTML)
	}
}

// Test that ParseMIMEMessage properly constructs a valid domain.Email
func TestParseMIMEMessage_DomainModelCompliance(t *testing.T) {
	raw := []byte("Subject: Test\nFrom: sender@example.com\n\nBody")

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	// Test that the email can be validated
	if err := email.Validate(); err != nil {
		t.Errorf("parsed email should be valid: %v", err)
	}
}

func TestParseMIMEMessage_AttachmentWithoutFilename(t *testing.T) {
	boundary := "boundary12345"
	raw := []byte(`Subject: Test
From: sender@example.com
Content-Type: multipart/mixed; boundary=` + boundary + `

--` + boundary + `
Content-Type: text/plain

Body
--` + boundary + `
Content-Disposition: attachment

Attachment content
--` + boundary + `--
`)

	email := ParseMIMEMessage("sender@example.com", []string{"recipient@example.com"}, raw)

	if len(email.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(email.Attachments))
	}
	if email.Attachments[0].Filename != "attachment" {
		t.Errorf("expected default filename 'attachment', got '%s'", email.Attachments[0].Filename)
	}
}
