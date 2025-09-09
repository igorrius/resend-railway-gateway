package domain

import (
	"errors"
	"strings"
)

// Email represents a normalized email message in the domain layer.
type Email struct {
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Text        string
	HTML        string
	ReplyTo     string
	Headers     map[string]string
	Attachments []Attachment
	Tags        []Tag
}

// Validate checks essential fields.
func (e Email) Validate() error {
	if strings.TrimSpace(e.From) == "" {
		return errors.New("from is required")
	}
	if len(e.To) == 0 {
		return errors.New("at least one recipient is required")
	}
	return nil
}

// NewEmail constructs an Email ensuring defaults and immutability of maps.
func NewEmail(from string, to []string, subject, text, html string, headers map[string]string) (Email, error) {
	normalizedTo := make([]string, 0, len(to))
	for _, r := range to {
		r = strings.TrimSpace(r)
		if r != "" {
			normalizedTo = append(normalizedTo, r)
		}
	}
	copiedHeaders := map[string]string{}
	for k, v := range headers {
		copiedHeaders[k] = v
	}
	e := Email{
		From:    strings.TrimSpace(from),
		To:      normalizedTo,
		Subject: subject,
		Text:    text,
		HTML:    html,
		Headers: copiedHeaders,
	}
	return e, e.Validate()
}

// Attachment represents a file attachment body.
type Attachment struct {
	Filename string
	Content  []byte
}

// Tag represents provider-specific metadata tags.
type Tag struct {
	Name  string
	Value string
}
