package resend

import (
	"github.com/igorrius/resend-railway-gateway/internal/domain"
	resendgo "github.com/resend/resend-go/v2"
)

type Client struct {
	client *resendgo.Client
}

func NewClient(apiKey string) *Client {
	return &Client{client: resendgo.NewClient(apiKey)}
}

func (c *Client) Send(email domain.Email) error {
	attachments := make([]*resendgo.Attachment, 0, len(email.Attachments))
	for _, a := range email.Attachments {
		attachments = append(attachments, &resendgo.Attachment{
			Filename: a.Filename,
			Content:  a.Content,
		})
	}
	tags := make([]resendgo.Tag, 0, len(email.Tags))
	for _, t := range email.Tags {
		tags = append(tags, resendgo.Tag{Name: t.Name, Value: t.Value})
	}
	request := &resendgo.SendEmailRequest{
		From:        email.From,
		To:          email.To,
		Cc:          email.Cc,
		Bcc:         email.Bcc,
		ReplyTo:     email.ReplyTo,
		Subject:     email.Subject,
		Html:        email.HTML,
		Text:        email.Text,
		Attachments: attachments,
		Tags:        tags,
		Headers:     email.Headers,
	}
	_, err := c.client.Emails.Send(request)
	return err
}

var _ domain.OutboundEmailSender = (*Client)(nil)
