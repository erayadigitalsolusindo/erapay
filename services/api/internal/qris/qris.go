// Package qris reads and rewrites EMVCo merchant-presented QR payloads as used by QRIS: it validates a merchant's
// STATIC QRIS (CRC and mandatory tags) and turns it into a DYNAMIC one with a fixed amount (tag 54), recomputing the
// CRC. Pure functions, no I/O. Nested templates (tags 26-51 etc.) are carried over untouched.
package qris

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrFormat    = errors.New("qris: malformed payload")
	ErrCRC       = errors.New("qris: CRC does not match")
	ErrNotQRIS   = errors.New("qris: not a QRIS payload")
	ErrNotStatic = errors.New("qris: not a static QRIS (it already has an amount or is dynamic)")
	ErrAmount    = errors.New("qris: amount out of range")
)

type tlv struct{ tag, val string }

func parse(p string) ([]tlv, error) {
	var out []tlv
	for i := 0; i < len(p); {
		if i+4 > len(p) {
			return nil, ErrFormat
		}
		tag := p[i : i+2]
		n, err := strconv.Atoi(p[i+2 : i+4])
		if err != nil || n < 0 || i+4+n > len(p) {
			return nil, ErrFormat
		}
		out = append(out, tlv{tag, p[i+4 : i+4+n]})
		i += 4 + n
	}
	return out, nil
}

func build(ts []tlv) string {
	var b strings.Builder
	for _, t := range ts {
		fmt.Fprintf(&b, "%s%02d%s", t.tag, len(t.val), t.val)
	}
	return b.String()
}

// CRC16 is CRC-16/CCITT-FALSE (poly 0x1021, init 0xFFFF) as 4 uppercase hex digits.
func CRC16(s string) string {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return fmt.Sprintf("%04X", crc)
}

// Info is what a tenant sees after a static QRIS is accepted.
type Info struct {
	MerchantName string
	City         string
}

// ValidateStatic checks a pasted/decoded static QRIS: well-formed TLV, correct CRC, payload format 01, static
// initiation (11), currency IDR (360), country ID, and no amount yet. The payload is trimmed first.
func ValidateStatic(payload string) (Info, error) {
	p := strings.TrimSpace(payload)
	ts, err := parse(p)
	if err != nil || len(ts) < 6 {
		return Info{}, ErrFormat
	}
	last := ts[len(ts)-1]
	if last.tag != "63" || len(last.val) != 4 {
		return Info{}, ErrFormat
	}
	if !strings.EqualFold(CRC16(p[:len(p)-4]), last.val) {
		return Info{}, ErrCRC
	}
	get := map[string]string{}
	for _, t := range ts {
		get[t.tag] = t.val
	}
	if get["00"] != "01" || get["53"] != "360" || get["58"] != "ID" || get["59"] == "" {
		return Info{}, ErrNotQRIS
	}
	if get["01"] != "11" {
		return Info{}, ErrNotStatic
	}
	if _, has := get["54"]; has {
		return Info{}, ErrNotStatic
	}
	return Info{MerchantName: get["59"], City: get["60"]}, nil
}

// Dynamic turns a valid static QRIS into one that asks for exactly amount rupiah: initiation 11 -> 12, tag 54 set
// (placed after the currency tag 53, as EMVCo orders tags), CRC recomputed.
func Dynamic(static string, amount int64) (string, error) {
	if amount <= 0 || amount > 9_999_999_999 {
		return "", ErrAmount
	}
	if _, err := ValidateStatic(static); err != nil {
		return "", err
	}
	ts, _ := parse(strings.TrimSpace(static))
	var out []tlv
	for _, t := range ts {
		switch t.tag {
		case "63":
			continue // rebuilt below
		case "01":
			t.val = "12"
		case "54":
			continue
		}
		out = append(out, t)
		if t.tag == "53" {
			out = append(out, tlv{"54", strconv.FormatInt(amount, 10)})
		}
	}
	body := build(out) + "6304"
	return body + CRC16(body), nil
}

// Amount returns the value of tag 54 of a payload (0 when absent), for tests and sanity checks.
func Amount(payload string) int64 {
	ts, err := parse(strings.TrimSpace(payload))
	if err != nil {
		return 0
	}
	for _, t := range ts {
		if t.tag == "54" {
			v, _ := strconv.ParseInt(t.val, 10, 64)
			return v
		}
	}
	return 0
}
