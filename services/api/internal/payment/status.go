// Package payment memuat aturan domain pembayaran yang tidak bergantung pada gateway mana pun.
package payment

import (
	"errors"
	"fmt"
)

type Status string

const (
	Created   Status = "created"
	Pending   Status = "pending"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Expired   Status = "expired"
	Cancelled Status = "cancelled"
	// Unknown = hasil ambigu (timeout dsb). BUKAN final: harus di-inquiry/rekonsiliasi (PRD §0 #3).
	Unknown Status = "unknown"
)

var ErrInvalidTransition = errors.New("payment: transisi status tidak diizinkan")

// allowed mengikuti PRD §6 (created→pending→final; pending→unknown→...).
// Tambahan dari invarian PRD §0 #3 dan kode error GATEWAY_REJECTED:
//   - created→unknown : create ke gateway timeout/ambigu
//   - created→failed  : gateway menolak secara pasti
//
// Status terminal tidak punya transisi keluar: callback telat tidak boleh membalik status.
var allowed = map[Status][]Status{
	Created: {Pending, Failed, Unknown},
	Pending: {Succeeded, Failed, Expired, Cancelled, Unknown},
	Unknown: {Pending, Succeeded, Failed, Expired},
}

func (s Status) IsTerminal() bool {
	switch s {
	case Succeeded, Failed, Expired, Cancelled:
		return true
	}
	return false
}

// Apply memeriksa perpindahan dari → ke.
// from == to dianggap tanpa perubahan (webhook duplikat aman, PRD §0 #5): (false, nil).
func Apply(from, to Status) (changed bool, err error) {
	if from == to {
		return false, nil
	}
	for _, next := range allowed[from] {
		if next == to {
			return true, nil
		}
	}
	return false, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, from, to)
}
