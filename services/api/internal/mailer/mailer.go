// Package mailer sends transactional email (verification, password reset).
//
// SMTP refuses to send credentials or message bodies over an unencrypted connection to a non-local
// host, so a misconfigured relay fails instead of leaking reset links. Log is for development only:
// it prints the whole message, links (and therefore tokens) included.
package mailer

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

type Message struct {
	To      string
	Subject string
	Body    string // plain text
}

type Mailer interface {
	Send(ctx context.Context, m Message) error
}

type SMTPConfig struct {
	Host, Port, User, Pass, From string
}

type SMTP struct{ cfg SMTPConfig }

func NewSMTP(cfg SMTPConfig) *SMTP {
	if cfg.Port == "" {
		cfg.Port = "587"
	}
	return &SMTP{cfg: cfg}
}

func hasCRLF(s string) bool { return strings.ContainsAny(s, "\r\n") }

func isLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return fmt.Errorf("mailer: invalid From: %w", err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil || hasCRLF(m.To) || hasCRLF(m.Subject) {
		return errors.New("mailer: invalid recipient or subject")
	}

	addr := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	d := net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.cfg.Port == "465" { // implicit TLS
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if s.cfg.Port != "465" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else if !isLocal(s.cfg.Host) {
			return errors.New("mailer: refusing to send over an unencrypted connection to a non-local host")
		}
	}
	if s.cfg.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.User, s.cfg.Pass, s.cfg.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	var id [8]byte
	_, _ = rand.Read(id[:])
	var body strings.Builder
	qp := quotedprintable.NewWriter(&body)
	_, _ = qp.Write([]byte(strings.ReplaceAll(m.Body, "\r\n", "\n")))
	_ = qp.Close()
	msg := "From: " + from.String() + "\r\n" +
		"To: " + to.String() + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", m.Subject) + "\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: <" + hex.EncodeToString(id[:]) + "@" + s.cfg.Host + ">\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" + body.String()
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Log prints messages instead of sending them. Development only.
type Log struct{ L *slog.Logger }

func (l Log) Send(_ context.Context, m Message) error {
	l.L.Warn("mailer (development: not sent)", "to", m.To, "subject", m.Subject, "body", m.Body)
	return nil
}

// Memory records messages (tests).
type Memory struct {
	mu    sync.Mutex
	sent  []Message
	Fail  error         // when set, Send returns it and records nothing
	Delay time.Duration // simulates a slow relay
}

func (m *Memory) Send(_ context.Context, msg Message) error {
	time.Sleep(m.Delay)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.sent = append(m.sent, msg)
	return nil
}

func (m *Memory) Sent() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.sent...)
}
