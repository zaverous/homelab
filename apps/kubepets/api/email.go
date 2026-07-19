package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// mailer sends transactional email (verification + password reset) through an
// SMTP relay - Resend in prod (smtp.resend.com:465, username "resend", the
// re_... API key as the password, all from the deployment env; the key alone
// lives in the SOPS secret).
//
// Port 465 is *implicit* TLS: the socket is wrapped in TLS from the very first
// byte. net/smtp only knows how to negotiate STARTTLS (the 587 style, plaintext
// then upgrade), so for 465 we dial the tls.Conn ourselves and hand the already
// -encrypted connection to smtp.NewClient.
type mailer struct {
	host     string // bare host, also the TLS ServerName + SMTP auth host
	addr     string // host:port
	username string
	password string
	from     string
	replyTo  string
}

// newMailer returns nil when SMTP isn't (fully) configured, mirroring
// newAuthService: the API still boots, and email-dependent flows fall back to
// logging the link instead of sending it (see (*app).sendVerifyEmail). That
// keeps local/dev usable with no relay, and keeps a missing secret from turning
// into a crash loop.
func newMailer() *mailer {
	host := os.Getenv("SMTP_HOST")
	user := os.Getenv("SMTP_USERNAME")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")
	unset := func(v string) bool { return v == "" || strings.HasPrefix(v, "REPLACE_ME") }
	if unset(host) || unset(user) || unset(pass) || unset(from) {
		log.Printf("mail disabled: SMTP_HOST/SMTP_USERNAME/SMTP_PASSWORD/SMTP_FROM not (fully) configured")
		return nil
	}
	return &mailer{
		host:     host,
		addr:     net.JoinHostPort(host, envOr("SMTP_PORT", "465")),
		username: user,
		password: pass,
		from:     from,
		replyTo:  os.Getenv("SMTP_REPLY_TO"),
	}
}

func (m *mailer) send(to, subject, body string) error {
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second}, "tcp", m.addr,
		&tls.Config{ServerName: m.host},
	)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer c.Close()

	if err := c.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.Mail(m.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := wc.Write([]byte(m.compose(to, subject, body))); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	return c.Quit()
}

// compose builds a minimal RFC 5322 message. Subjects here are plain ASCII, so
// no MIME word-encoding is needed. CRLF line endings are required by SMTP.
func (m *mailer) compose(to, subject, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", m.from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	if m.replyTo != "" {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", m.replyTo)
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}
