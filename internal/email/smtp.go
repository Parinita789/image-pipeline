package email

import (
	"fmt"
	"net/smtp"
)

type Service struct {
	host        string
	port        string
	username    string
	password    string
	fromEmail   string
	frontendURL string
}

func NewService(host, port, username, password, fromEmail, frontendURL string) *Service {
	return &Service{
		host:        host,
		port:        port,
		username:    username,
		password:    password,
		fromEmail:   fromEmail,
		frontendURL: frontendURL,
	}
}

func (s *Service) SendPasswordResetEmail(toEmail, token string) error {
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, token)

	subject := "Reset your Pixelift password"
	htmlBody := fmt.Sprintf(`<div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:24px">
<h2 style="color:#1a1a1a">Reset your password</h2>
<p>You requested a password reset for your Pixelift account.</p>
<p>Click the button below to set a new password. This link expires in <strong>15 minutes</strong>.</p>
<a href="%s" style="display:inline-block;padding:12px 24px;background:#2563eb;color:#fff;text-decoration:none;border-radius:6px;margin:16px 0">Reset Password</a>
<p style="color:#666;font-size:13px">Or copy this link: %s</p>
<hr style="border:none;border-top:1px solid #eee;margin:24px 0"/>
<p style="color:#999;font-size:12px">If you didn't request this, ignore this email.</p>
</div>`, resetLink, resetLink)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s",
		s.fromEmail, toEmail, subject, htmlBody)

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	addr := fmt.Sprintf("%s:%s", s.host, s.port)

	return smtp.SendMail(addr, auth, s.fromEmail, []string{toEmail}, []byte(msg))
}
