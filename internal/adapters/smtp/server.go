package smtp

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"

	"github.com/emersion/go-sasl"
	goSMTP "github.com/emersion/go-smtp"
	"github.com/igorrius/resend-railway-gateway/internal/app"
	"github.com/igorrius/resend-railway-gateway/internal/domain"
)

// Options configures the SMTP listener's access controls.
type Options struct {
	// RequireAuth advertises AUTH PLAIN and rejects MAIL FROM until the client
	// has authenticated. Keeps the relay closed by default.
	RequireAuth bool
	// Username and Password are the credentials accepted over AUTH PLAIN.
	Username string
	Password string
	// AllowedFromDomains, when non-empty, restricts the envelope MAIL FROM domain
	// to this allowlist (lower-cased).
	AllowedFromDomains []string
}

// ErrServerClosed is returned by Shutdown when the server was already closed.
// Re-exported so callers depend only on this package.
var ErrServerClosed = goSMTP.ErrServerClosed

var errSenderDomainNotAllowed = &goSMTP.SMTPError{
	Code:         550,
	EnhancedCode: goSMTP.EnhancedCode{5, 7, 1},
	Message:      "sender domain not allowed",
}

// Session implements go-smtp's Session interface to handle SMTP protocol operations.
// It collects email data during the SMTP conversation and sends it through the service.
type Session struct {
	service       *app.Service
	opts          Options
	authenticated bool
	mailFrom      string
	rcpts         []string
	data          bytes.Buffer
}

func (s *Session) Reset()        { s.mailFrom = ""; s.rcpts = nil; s.data.Reset() }
func (s *Session) Logout() error { return nil }

// AuthMechanisms advertises supported SASL mechanisms. PLAIN is offered only
// when authentication is required (credentials are configured).
func (s *Session) AuthMechanisms() []string {
	if s.opts.RequireAuth {
		return []string{sasl.Plain}
	}
	return nil
}

// Auth returns a SASL server for the requested mechanism, validating credentials
// and flipping the session to authenticated on success.
func (s *Session) Auth(mech string) (sasl.Server, error) {
	if !s.opts.RequireAuth {
		return nil, goSMTP.ErrAuthUnsupported
	}
	if mech != sasl.Plain {
		return nil, goSMTP.ErrAuthUnknownMechanism
	}
	return sasl.NewPlainServer(func(_, username, password string) error {
		if !credentialsMatch(username, password, s.opts.Username, s.opts.Password) {
			return goSMTP.ErrAuthFailed
		}
		s.authenticated = true
		return nil
	}), nil
}

func (s *Session) Mail(from string, _ *goSMTP.MailOptions) error {
	if s.opts.RequireAuth && !s.authenticated {
		return goSMTP.ErrAuthRequired
	}
	if !senderDomainAllowed(from, s.opts.AllowedFromDomains) {
		return errSenderDomainNotAllowed
	}
	s.mailFrom = from
	return nil
}

func (s *Session) Rcpt(to string, _ *goSMTP.RcptOptions) error {
	s.rcpts = append(s.rcpts, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	s.data.Reset()
	if _, err := io.Copy(&s.data, r); err != nil {
		return err
	}
	email := ParseMIMEMessage(s.mailFrom, s.rcpts, s.data.Bytes())
	if err := s.service.HandleEmail(email); err != nil {
		return toSMTPError(err)
	}
	return nil
}

// toSMTPError maps a send failure to an SMTP reply: transient failures become a
// 4xx (temporary) so the client retries, permanent failures become a 5xx.
func toSMTPError(err error) error {
	if domain.IsTransient(err) {
		return &goSMTP.SMTPError{
			Code:         451,
			EnhancedCode: goSMTP.EnhancedCode{4, 3, 0},
			Message:      "temporary failure delivering to upstream, please retry",
		}
	}
	return &goSMTP.SMTPError{
		Code:         554,
		EnhancedCode: goSMTP.EnhancedCode{5, 3, 0},
		Message:      "message rejected",
	}
}

// credentialsMatch compares credentials in constant time.
func credentialsMatch(user, pass, wantUser, wantPass string) bool {
	u := subtle.ConstantTimeCompare([]byte(user), []byte(wantUser))
	p := subtle.ConstantTimeCompare([]byte(pass), []byte(wantPass))
	return u == 1 && p == 1
}

// senderDomainAllowed reports whether the MAIL FROM domain passes the allowlist.
// An empty allowlist permits every sender.
func senderDomainAllowed(from string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	d := addressDomain(from)
	for _, a := range allowed {
		if d == a {
			return true
		}
	}
	return false
}

// addressDomain extracts the lower-cased domain from an SMTP address, tolerating
// angle brackets.
func addressDomain(addr string) string {
	addr = strings.Trim(strings.TrimSpace(addr), "<> ")
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return strings.ToLower(addr[at+1:])
}

// Backend implements go-smtp Backend to provide SMTP server functionality.
type Backend struct {
	service *app.Service
	opts    Options
}

func (b *Backend) NewSession(_ *goSMTP.Conn) (goSMTP.Session, error) {
	return &Session{service: b.service, opts: b.opts}, nil
}

// NewServer creates and configures a new SMTP server.
// - addr: Listen address (e.g., ":2525")
// - service: Application service for handling emails
// - opts: Access-control options (AUTH, sender allowlist)
func NewServer(addr string, service *app.Service, opts Options) *goSMTP.Server {
	backend := &Backend{service: service, opts: opts}
	s := goSMTP.NewServer(backend)
	s.Addr = addr
	s.Domain = "localhost"
	// AUTH PLAIN is transmitted over the (private-network) connection without
	// TLS; the calling service reaches this listener over Railway's private
	// network, so insecure AUTH is permitted here.
	s.AllowInsecureAuth = true
	return s
}

var _ goSMTP.AuthSession = (*Session)(nil)

// ParseMIMEMessage performs a lightweight parse of headers and common MIME structures.
// It supports:
// - Simple text/plain and text/html messages
// - Multipart messages (alternative and mixed)
// - Nested multipart messages
// - Base64 and quoted-printable encoding
// - Attachments (inline and regular)
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

		// Parse content type to determine if this is text/plain, text/html, or nested multipart
		mediatype, params, err := mime.ParseMediaType(pctype)

		// Check if this is an attachment
		// Some clients (like Vaultwarden) put the filename in the Content-Type header
		filename := part.FileName()
		if filename == "" && err == nil {
			filename = params["name"]
		}

		contentID := part.Header.Get("Content-ID")

		isAttachment := strings.HasPrefix(lowerDisp, "attachment") ||
			(strings.HasPrefix(lowerDisp, "inline") && (filename != "" || contentID != "")) ||
			filename != "" ||
			contentID != ""

		if isAttachment {
			if filename == "" {
				if contentID != "" {
					filename = strings.Trim(contentID, "<>")
				} else {
					filename = "attachment"
				}
			}

			attachments = append(attachments, domain.Attachment{
				Filename:    filename,
				Content:     slurp,
				ContentType: pctype,
				ContentID:   contentID,
			})
		} else {
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
