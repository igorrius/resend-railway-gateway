package main

import (
	"log"
	"os"
	"time"

	resendclient "github.com/igorrius/resend-railway-gateway/internal/adapters/resend"
	smtpserver "github.com/igorrius/resend-railway-gateway/internal/adapters/smtp"
	"github.com/igorrius/resend-railway-gateway/internal/app"
	"github.com/igorrius/resend-railway-gateway/internal/config"
)

type stdLogger struct{}

func (l stdLogger) Info(msg string, fields map[string]any)  { log.Printf("INFO %s %v", msg, fields) }
func (l stdLogger) Error(msg string, fields map[string]any) { log.Printf("ERROR %s %v", msg, fields) }

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	// optional override for Railway dynamic ports
	if v := os.Getenv("PORT"); v != "" {
		cfg.SMTPListerAddr = ":" + v
	}

	sender := resendclient.NewClient(cfg.ResendAPIKey)
	svc := app.NewService(sender, stdLogger{}, cfg.SendTimeout)
	server := smtpserver.NewServer(cfg.SMTPListerAddr, svc)
	log.Printf("SMTP gateway listening on %s", cfg.SMTPListerAddr)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
	_ = time.Now()
}
