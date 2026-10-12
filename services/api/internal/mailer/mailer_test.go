package mailer

import (
	"bufio"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"testing"
)

// fakeSMTP is a minimal plaintext SMTP server (no STARTTLS) that records the DATA section.
func fakeSMTP(t *testing.T) (port string, data <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	out := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		say("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
				say("250 fake")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				say("354 go")
				var b strings.Builder
				for {
					l, _ := r.ReadString('\n')
					if l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				out <- b.String()
				say("250 queued")
			case cmd == "QUIT":
				say("221 bye")
				return
			}
		}
	}()
	_, port, _ = net.SplitHostPort(ln.Addr().String())
	return port, out
}

func TestSMTPSendsWellFormedUTF8Message(t *testing.T) {
	port, data := fakeSMTP(t)
	s := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: port, From: "EraPay <no-reply@erapay.test>"})
	body := "Halo,\nKonfirmasi email Anda: https://app.erapay.test/verify-email?token=abc_DEF-123\n"
	if err := s.Send(t.Context(), Message{To: "budi@toko.test", Subject: "Konfirmasi email Anda – EraPay", Body: body}); err != nil {
		t.Fatal(err)
	}
	m, err := mail.ReadMessage(strings.NewReader(<-data))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject")); got != "Konfirmasi email Anda – EraPay" {
		t.Fatalf("subject: %q", got)
	}
	if m.Header.Get("To") != "<budi@toko.test>" || !strings.Contains(m.Header.Get("From"), "no-reply@erapay.test") {
		t.Fatalf("headers: %v", m.Header)
	}
	raw, _ := io.ReadAll(quotedprintable.NewReader(m.Body))
	if strings.ReplaceAll(string(raw), "\r\n", "\n") != body {
		t.Fatalf("body: %q", raw)
	}
}

func TestSMTPRejectsHeaderInjection(t *testing.T) {
	s := NewSMTP(SMTPConfig{Host: "127.0.0.1", Port: "1", From: "no-reply@erapay.test"})
	for name, m := range map[string]Message{
		"subject": {To: "a@b.test", Subject: "x\r\nBcc: victim@evil.test", Body: "b"},
		"to":      {To: "a@b.test\r\nBcc: victim@evil.test", Subject: "x", Body: "b"},
		"no @":    {To: "not-an-address", Subject: "x", Body: "b"},
	} {
		// Port 1 is closed: reaching the network would be a different error, so check the message is refused first.
		if err := s.Send(t.Context(), m); err == nil || !strings.Contains(err.Error(), "invalid recipient or subject") {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestPlaintextOnlyAllowedForLoopback(t *testing.T) {
	for host, want := range map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true, "smtp.example.com": false, "10.0.0.5": false} {
		if isLocal(host) != want {
			t.Fatalf("%s: %v", host, !want)
		}
	}
}
