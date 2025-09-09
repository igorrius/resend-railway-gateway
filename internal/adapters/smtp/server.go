package smtp

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"

	goSMTP "github.com/emersion/go-smtp"
	"github.com/igorrius/resend-railway-gateway/internal/app"
	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

// Session implements go-smtp's Session interface.
type Session struct {
	service  *app.Service
	mailFrom string
	rcpts    []string
	data     bytes.Buffer
}

func (s *Session) Reset()        { s.mailFrom = ""; s.rcpts = nil; s.data.Reset() }
func (s *Session) Logout() error { return nil }

func (s *Session) Mail(from string, _ *goSMTP.MailOptions) error {
	s.mailFrom = from
	return nil
}

func (s *Session) Rcpt(to string, _ *goSMTP.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	s.data.Reset()
	_, err := io.Copy(&s.data, r)
	if err != nil {
		return err
	}
	email := ParseMIMEMessage(s.mailFrom, s.rcpts, s.data.Bytes())
	return s.service.HandleEmail(email)
}

// Backend implements go-smtp Backend.
type Backend struct{ service *app.Service }

func (b *Backend) NewSession(_ *goSMTP.Conn) (goSMTP.Session, error) {
	return &Session{service: b.service}, nil
}

func NewServer(addr string, service *app.Service) *goSMTP.Server {
	backend := &Backend{service: service}
	s := goSMTP.NewServer(backend)
	s.Addr = addr
	s.Domain = "localhost"
	s.AllowInsecureAuth = true
	return s
}

// ParseMIMEMessage performs a lightweight parse of headers and common MIME structures.
func ParseMIMEMessage(from string, rcpts []string, raw []byte) domain.Email {
	headers := map[string]string{}
	subject := ""
	textBody := ""
	htmlBody := ""
	var cc []string
	var bcc []string
	replyTo := ""
	attachments := make([]domain.Attachment, 0, 4)

	mr := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw)))
	hdr, _ := mr.ReadMIMEHeader()
	if hdr != nil {
		for k, v := range hdr {
			if len(v) == 0 {
				continue
			}
			headers[k] = v[0]
		}
		subject = hdr.Get("Subject")
		if v := hdr.Get("Cc"); v != "" {
			cc = splitAddrs(v)
		}
		if v := hdr.Get("Bcc"); v != "" {
			bcc = splitAddrs(v)
		}
		if v := hdr.Get("Reply-To"); v != "" {
			replyTo = v
		}

		ct := hdr.Get("Content-Type")
		mediatype, params, err := mime.ParseMediaType(ct)
		if err == nil && strings.HasPrefix(mediatype, "multipart/") {
			boundary := params["boundary"]
			bodyStart := bytes.Index(raw, []byte("\r\n\r\n"))
			if bodyStart < 0 {
				bodyStart = bytes.Index(raw, []byte("\n\n"))
			}
			if bodyStart >= 0 {
				bodyData := raw[bodyStart+2:]
				textBody, htmlBody, attachments = parseMultipartBody(bodyData, boundary, attachments)
			}
		} else {
			// Handle non-multipart emails
			bodyStart := bytes.Index(raw, []byte("\r\n\r\n"))
			if bodyStart < 0 {
				bodyStart = bytes.Index(raw, []byte("\n\n"))
			}
			if bodyStart >= 0 {
				bodyData := raw[bodyStart+2:]
				cte := strings.ToLower(hdr.Get("Content-Transfer-Encoding"))
				var reader io.Reader = bytes.NewReader(bodyData)
				switch cte {
				case "base64":
					reader = base64.NewDecoder(base64.StdEncoding, bytes.NewReader(bodyData))
				case "quoted-printable":
					reader = quotedprintable.NewReader(bytes.NewReader(bodyData))
				}
				slurp, _ := io.ReadAll(reader)
				if strings.HasPrefix(strings.ToLower(mediatype), "text/plain") {
					textBody = string(slurp)
				} else if strings.HasPrefix(strings.ToLower(mediatype), "text/html") {
					htmlBody = string(slurp)
				} else {
					// Default to text if content type is not specified
					textBody = string(slurp)
				}
			}
		}
	}
	email, _ := domain.NewEmail(from, append([]string(nil), rcpts...), subject, textBody, htmlBody, headers)
	email.Cc = cc
	email.Bcc = bcc
	email.ReplyTo = replyTo
	email.Attachments = attachments
	return email
}

// parseMultipartBody recursively parses multipart content, handling nested multipart structures
func parseMultipartBody(bodyData []byte, boundary string, attachments []domain.Attachment) (string, string, []domain.Attachment) {
	textBody := ""
	htmlBody := ""

	mpr := multipart.NewReader(bytes.NewReader(bodyData), boundary)
	for {
		part, err := mpr.NextPart()
		if err != nil {
			break
		}

		// Handle content transfer encoding
		var reader io.Reader = part
		cte := strings.ToLower(part.Header.Get("Content-Transfer-Encoding"))
		switch cte {
		case "base64":
			reader = base64.NewDecoder(base64.StdEncoding, part)
		case "quoted-printable":
			reader = quotedprintable.NewReader(part)
		}

		// Read the part content
		slurp, _ := io.ReadAll(reader)
		disp := part.Header.Get("Content-Disposition")
		pctype := part.Header.Get("Content-Type")
		lowerDisp := strings.ToLower(disp)

		// Check if this is an attachment
		if strings.HasPrefix(lowerDisp, "attachment") || (strings.HasPrefix(lowerDisp, "inline") && part.FileName() != "") {
			filename := part.FileName()
			if filename == "" {
				filename = "attachment"
			}
			attachments = append(attachments, domain.Attachment{Filename: filename, Content: slurp})
		} else {
			// Parse content type to determine if this is text/plain, text/html, or nested multipart
			mediatype, params, err := mime.ParseMediaType(pctype)
			if err == nil && strings.HasPrefix(mediatype, "multipart/") {
				// This is a nested multipart, recurse
				nestedBoundary := params["boundary"]
				var nestedText, nestedHtml string
				nestedText, nestedHtml, attachments = parseMultipartBody(slurp, nestedBoundary, attachments)
				if textBody == "" {
					textBody = nestedText
				}
				if htmlBody == "" {
					htmlBody = nestedHtml
				}
			} else if strings.HasPrefix(strings.ToLower(pctype), "text/plain") {
				textBody = string(slurp)
			} else if strings.HasPrefix(strings.ToLower(pctype), "text/html") {
				htmlBody = string(slurp)
			}
		}
	}

	return textBody, htmlBody, attachments
}

func splitAddrs(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
