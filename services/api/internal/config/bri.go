package config

import (
	"errors"
	"os"
	"strings"
)

// BRI holds Bank BRI (BRIAPI SNAP) credentials, read ONLY from the environment. Nothing here is logged.
type BRI struct {
	BaseURL       string // empty = adapter default (sandbox)
	ClientID      string
	ClientSecret  string
	PrivateKeyPEM []byte // BRI_PRIVATE_KEY (PEM; literal \n accepted) or BRI_PRIVATE_KEY_FILE
	PartnerID     string
	ChannelID     string
	MerchantID    string
	TerminalID    string
}

// LoadBRI returns nil when BRI_CLIENT_ID is unset: the gateway is simply not registered.
func LoadBRI() (*BRI, error) {
	id := os.Getenv("BRI_CLIENT_ID")
	if id == "" {
		return nil, nil
	}
	b := &BRI{
		BaseURL: os.Getenv("BRI_BASE_URL"), ClientID: id, ClientSecret: os.Getenv("BRI_CLIENT_SECRET"),
		PartnerID: os.Getenv("BRI_PARTNER_ID"), ChannelID: os.Getenv("BRI_CHANNEL_ID"),
		MerchantID: os.Getenv("BRI_MERCHANT_ID"), TerminalID: os.Getenv("BRI_TERMINAL_ID"),
	}
	switch {
	case os.Getenv("BRI_PRIVATE_KEY") != "":
		b.PrivateKeyPEM = []byte(strings.ReplaceAll(os.Getenv("BRI_PRIVATE_KEY"), `\n`, "\n"))
	case os.Getenv("BRI_PRIVATE_KEY_FILE") != "":
		k, err := os.ReadFile(os.Getenv("BRI_PRIVATE_KEY_FILE"))
		if err != nil {
			return nil, errors.New("BRI_PRIVATE_KEY_FILE: cannot read file")
		}
		b.PrivateKeyPEM = k
	default:
		return nil, errors.New("BRI_CLIENT_ID is set but BRI_PRIVATE_KEY / BRI_PRIVATE_KEY_FILE is missing")
	}
	return b, nil
}
