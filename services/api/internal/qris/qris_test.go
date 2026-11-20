package qris

import (
	"errors"
	"strings"
	"testing"
)

// sample builds a synthetic static QRIS (no real merchant): nested tag 26 with a made-up GUID, MCC, IDR, ID.
func sample(initiation string, extra ...tlv) string {
	ts := []tlv{{"00", "01"}, {"01", initiation}, {"26", "0016ID.CO.EXAMPLE.WWW01189360000000000000001"}, {"52", "5812"}, {"53", "360"}}
	ts = append(ts, extra...)
	ts = append(ts, tlv{"58", "ID"}, tlv{"59", "TOKO CONTOH"}, tlv{"60", "JAKARTA"})
	body := build(ts) + "6304"
	return body + CRC16(body)
}

func TestCRC16KnownVector(t *testing.T) {
	// CRC-16/CCITT-FALSE check value for "123456789" is 29B1.
	if got := CRC16("123456789"); got != "29B1" {
		t.Fatalf("CRC16 = %s; want 29B1", got)
	}
}

func TestValidateStatic(t *testing.T) {
	info, err := ValidateStatic(sample("11"))
	if err != nil || info.MerchantName != "TOKO CONTOH" || info.City != "JAKARTA" {
		t.Fatalf("valid static: %+v %v", info, err)
	}
	if _, err := ValidateStatic("  " + sample("11") + "\n"); err != nil {
		t.Fatalf("surrounding whitespace must be ignored: %v", err)
	}
	good := sample("11")
	for name, c := range map[string]struct {
		in   string
		want error
	}{
		"empty":              {"", ErrFormat},
		"garbage":            {"hello world", ErrFormat},
		"bad crc":            {good[:len(good)-1] + "0", ErrCRC},
		"tampered body":      {strings.Replace(good, "TOKO", "TOKX", 1), ErrCRC},
		"dynamic already":    {sample("12"), ErrNotStatic},
		"static with amount": {sample("11", tlv{"54", "10000"}), ErrNotStatic},
	} {
		if _, err := ValidateStatic(c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v; want %v", name, err, c.want)
		}
	}
	// Not IDR / not Indonesia is not QRIS.
	notID := strings.Replace(strings.TrimSuffix(good, good[len(good)-4:]), "5802ID", "5802SG", 1)
	notID += CRC16(notID)
	if _, err := ValidateStatic(notID); !errors.Is(err, ErrNotQRIS) {
		t.Errorf("country SG: %v", err)
	}
}

func TestDynamicSetsAmountKeepsEverythingElseAndFixesCRC(t *testing.T) {
	static := sample("11")
	d, err := Dynamic(static, 25007)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := parse(d)
	if err != nil {
		t.Fatal(err)
	}
	get := map[string]string{}
	var order []string
	for _, x := range ts {
		get[x.tag] = x.val
		order = append(order, x.tag)
	}
	if get["01"] != "12" || get["54"] != "25007" || Amount(d) != 25007 || get["59"] != "TOKO CONTOH" || get["26"] != "0016ID.CO.EXAMPLE.WWW01189360000000000000001" {
		t.Fatalf("dynamic fields: %v", get)
	}
	if strings.Join(order, ",") != "00,01,26,52,53,54,58,59,60,63" {
		t.Fatalf("tag order %v; amount must follow the currency tag", order)
	}
	if want := CRC16(d[:len(d)-4]); !strings.HasSuffix(d, want) {
		t.Fatalf("CRC not recomputed: %s", d)
	}
	// Two amounts from the same static differ only in tag 54 and CRC.
	d2, _ := Dynamic(static, 25008)
	if d == d2 || !strings.Contains(d2, "540525008") {
		t.Fatalf("second amount: %s", d2)
	}
	for _, bad := range []int64{0, -1, 10_000_000_000} {
		if _, err := Dynamic(static, bad); !errors.Is(err, ErrAmount) {
			t.Errorf("amount %d: %v", bad, err)
		}
	}
	if _, err := Dynamic(sample("12"), 1000); !errors.Is(err, ErrNotStatic) {
		t.Errorf("dynamic input: %v", err)
	}
}
