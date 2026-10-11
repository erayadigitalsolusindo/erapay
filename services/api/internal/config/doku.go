package config

import (
	"errors"
	"os"
	"strings"
)

// DOKU holds DOKU credentials, read ONLY from the environment. Nothing here is ever logged.
type DOKU struct {
	ClientID           string
	ClientSecret       string
	PrivateKeyPEM      []byte // DOKU_PRIVATE_KEY (PEM; literal \n accepted) or DOKU_PRIVATE_KEY_FILE
	TerminalID         string
	PostalCode         string
	BaseURL            string // empty = adapter default (sandbox)
	StatusMapConfirmed bool   // DOKU_QRIS_STATUS_MAP_CONFIRMED=true after verifying QRIS status codes in the sandbox
}

// LoadDOKU returns nil when DOKU_CLIENT_ID is unset: the gateway is simply not registered
// and payments report PAYMENT_METHOD_UNAVAILABLE.
func LoadDOKU() (*DOKU, error) {
	id := os.Getenv("DOKU_CLIENT_ID")
	if id == "" {
		return nil, nil
	}
	d := &DOKU{
		ClientID: id, ClientSecret: os.Getenv("DOKU_SECRET_KEY"),
		TerminalID: os.Getenv("DOKU_TERMINAL_ID"), PostalCode: os.Getenv("DOKU_POSTAL_CODE"),
		BaseURL:            os.Getenv("DOKU_BASE_URL"),
		StatusMapConfirmed: os.Getenv("DOKU_QRIS_STATUS_MAP_CONFIRMED") == "true",
	}
	switch {
	case os.Getenv("DOKU_PRIVATE_KEY") != "":
		d.PrivateKeyPEM = []byte(strings.ReplaceAll(os.Getenv("DOKU_PRIVATE_KEY"), `\n`, "\n"))
	case os.Getenv("DOKU_PRIVATE_KEY_FILE") != "":
		b, err := os.ReadFile(os.Getenv("DOKU_PRIVATE_KEY_FILE"))
		if err != nil {
			return nil, errors.New("DOKU_PRIVATE_KEY_FILE: cannot read file")
		}
		d.PrivateKeyPEM = b
	default:
		return nil, errors.New("DOKU_CLIENT_ID is set but DOKU_PRIVATE_KEY / DOKU_PRIVATE_KEY_FILE is missing")
	}
	return d, nil
}
