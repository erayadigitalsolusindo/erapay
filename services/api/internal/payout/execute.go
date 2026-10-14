package payout

// The gateway-driven leg of a payout (PRD payout.md, doku.md §6, decision 14 Okt 2026).
//
// Execute runs the documented flow for an approved payout: account inquiry (mandatory
// before a transfer, partner-paid per success) then the transfer. The payout ID is the
// gateway partner reference, which makes the flow safe against crashes: if we stop
// between the claim and the apply, a retry cannot double-pay — the gateway answers a
// repeated partner reference with a duplicate result, the payout lands in 'unknown',
// and CheckStatus resolves it (PRD §0 #3: an ambiguous outcome is never released and
// never retried blindly).
//
// State machine: approved -> submitted -> paid | failed | returned (via CheckStatus).
// An ambiguous outcome is 'unknown'. Only Finance resolves those: CheckStatus, or a
// manual MarkPaid after a bank statement confirms the funds arrived.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// validate checks the sender identity the gateway requires for every transfer.
func (s Sender) validate() error {
	if s.CustomerNumber == "" || s.FirstName == "" || s.LastName == "" || s.PersonalID == "" || s.IDType == "" {
		return fmt.Errorf("%w: payout sender identity (phone, first/last name, id, id type) must be set in config", ErrValidation)
	}
	return nil
}

// Execute moves an approved payout through the gateway. Idempotent per payout: a second
// call on the same payout fails with ErrState, and the gateway dedupes the partner
// reference anyway. Requires the executor gateway (SetDisburser) and the live gate.
func (s *Service) Execute(ctx context.Context, id string, a Actor) (Payout, error) {
	if s.dis == nil {
		return Payout{}, ErrNoDisburser
	}
	if !s.cfg.DryRun && !s.cfg.LiveEnabled {
		return Payout{}, ErrDisabled
	}
	p, d, err := s.loadForExecution(ctx, id, "approved")
	if err != nil {
		return Payout{}, err
	}
	if s.cfg.DryRun {
		// Non-production: no gateway call, no money. Record the submit so the rest of
		// the machine can be exercised; Finance confirms with MarkPaid as usual.
		if s.cfg.FeeFlat >= p.Amount {
			return Payout{}, fmt.Errorf("%w: configured fee %d leaves nothing to pay out", ErrValidation, s.cfg.FeeFlat)
		}
		return s.claimSubmit(ctx, id, "dry-run", s.cfg.FeeFlat, a)
	}
	if err := s.cfg.Sender.validate(); err != nil {
		return Payout{}, err
	}
	// 1. Account inquiry: validates the destination and returns the session id the
	// transfer requires plus the fee the gateway will charge for it.
	inquiry, err := s.dis.InquiryBankAccount(ctx, gateway.BankInquiryRequest{
		CustomerNumber: s.cfg.Sender.CustomerNumber,
		AccountNo:      d.accountNo,
		AccountName:    d.accountName,
		BankCode:       d.bankCode,
		Amount:         p.Amount,
		Currency:       p.Currency,
		SenderCountry:  s.cfg.Sender.Country,
		ChannelCode:    s.cfg.ChannelCode,
	})
	if err != nil {
		// The transfer needs the inquiry's session id, so nothing was sent: the payout
		// stays 'approved' and the operator can fix the cause and execute again.
		return Payout{}, err
	}
	if msg := nameMismatch(d.accountName, inquiry.AccountName); msg != "" {
		if _, ferr := s.MarkFailed(ctx, id, msg, a); ferr != nil {
			return Payout{}, ferr
		}
		return Payout{}, fmt.Errorf("%w: %s", ErrNameMismatch, msg)
	}
	fee := inquiry.FeeAmount
	if fee < 0 {
		fee = s.cfg.FeeFlat
	}
	if fee >= p.Amount {
		reason := fmt.Sprintf("transfer fee (%d) meets or exceeds the payout amount (%d)", fee, p.Amount)
		if _, ferr := s.MarkFailed(ctx, id, reason, a); ferr != nil {
			return Payout{}, ferr
		}
		return Payout{}, fmt.Errorf("%w: %s", ErrValidation, reason)
	}
	// 2. Claim: approved -> submitted, atomically. A second Execute on the same payout
	// loses here (ErrState) before any transfer is sent.
	p, err = s.claimSubmit(ctx, id, s.dis.Code(), fee, a)
	if err != nil {
		return Payout{}, err
	}
	// 3. Transfer. The beneficiary receives the payout nominal minus the fee.
	res, err := s.dis.TransferBank(ctx, gateway.TransferRequest{
		Reference:              id,
		CustomerNumber:         s.cfg.Sender.CustomerNumber,
		SessionID:              inquiry.SessionID,
		AccountNo:              d.accountNo,
		BankCode:               d.bankCode,
		Amount:                 p.Amount - fee,
		Currency:               p.Currency,
		BeneficiaryFirstName:   d.beneficiaryFirstName,
		BeneficiaryLastName:    d.beneficiaryLastName,
		BeneficiaryPhone:       d.beneficiaryPhone,
		BeneficiaryAccountName: d.accountName,
		SenderCountry:          s.cfg.Sender.Country,
		SenderFirstName:        s.cfg.Sender.FirstName,
		SenderLastName:         s.cfg.Sender.LastName,
		SenderPersonalID:       s.cfg.Sender.PersonalID,
		SenderIDType:           s.cfg.Sender.IDType,
		ChannelCode:            s.cfg.ChannelCode,
	})
	if err != nil {
		if gateway.IsAmbiguous(err) {
			if uerr := s.markUnknown(ctx, id, "gateway outcome unknown after transfer: "+truncate(err.Error(), 480), a); uerr != nil {
				return Payout{}, uerr
			}
			// The caller still gets the ambiguity: it must not treat this as success.
			return Payout{}, err
		}
		// Definitive rejection: the transfer did not happen, the funds return to the merchant.
		if _, ferr := s.MarkFailed(ctx, id, truncate(err.Error(), 480), a); ferr != nil {
			return Payout{}, ferr
		}
		return Payout{}, err
	}
	// Store the gateway reference: it is the bank reference for the eventual paid state.
	if err := s.storeGatewayReference(ctx, id, res.GatewayReference, p, a); err != nil {
		return Payout{}, err
	}
	return s.GetAny(ctx, id)
}

// CheckStatus asks the gateway for the outcome of a submitted or unknown payout and
// applies a definitive one: paid, failed (funds released) or returned (funds released).
// Pending and unmapped statuses change nothing, as does an ambiguous error: the payout
// is left for the next check or for Finance.
func (s *Service) CheckStatus(ctx context.Context, id string, a Actor) (Payout, error) {
	if s.dis == nil {
		return Payout{}, ErrNoDisburser
	}
	p, d, err := s.loadForExecution(ctx, id, "submitted", "unknown")
	if err != nil {
		return Payout{}, err
	}
	if p.Gateway == "dry-run" {
		return Payout{}, ErrState
	}
	res, err := s.dis.TransferStatus(ctx, gateway.TransferStatusRequest{
		OriginalPartnerReference: id,
		OriginalReference:        refOr(p.BankReference),
		AccountNo:                d.accountNo,
		BankCode:                 d.bankCode,
		ServiceCode:              s.cfg.ServiceCode,
	})
	if err != nil {
		return p, err
	}
	st, ok := s.dis.MapTransferStatus(res.RawStatus)
	if !ok {
		// Not "none of our business": an unmapped status means the mapping is unconfirmed,
		// so nothing changes but ops should see what came back.
		if aerr := s.auditCheckStatus(ctx, id, a, "unmapped", map[string]any{"raw_status": res.RawStatus}); aerr != nil {
			return p, aerr
		}
		return p, nil
	}
	switch st {
	case gateway.TransferSucceeded:
		ref := refOr(p.BankReference)
		if ref == "" {
			// The transfer was accepted but the response never reached us ('unknown'); the
			// gateway's own reference arrives only with the transfer response, so a
			// traceable synthetic one records who said it succeeded.
			ref = s.dis.Code() + ":" + id
		}
		return s.MarkPaid(ctx, id, ref, a)
	case gateway.TransferFailed:
		return s.end(ctx, id, "", []string{"submitted", "unknown"}, "failed",
			truncate(fmt.Sprintf("gateway reports the transfer failed (%s)", res.RawStatus), 480), a, "payout.check_status")
	case gateway.TransferRefunded:
		return s.end(ctx, id, "", []string{"submitted", "unknown"}, "returned",
			truncate(fmt.Sprintf("gateway reports the transfer was returned (%s)", res.RawStatus), 480), a, "payout.check_status")
	default: // pending or anything else: unchanged
		if aerr := s.auditCheckStatus(ctx, id, a, string(st), map[string]any{"raw_status": res.RawStatus}); aerr != nil {
			return p, aerr
		}
		return p, nil
	}
}

// destination holds the decrypted payout destination and the beneficiary fields the
// gateway flow needs. Loaded in a short transaction; the gateway calls happen after it.
type destination struct {
	bankCode             string
	accountNo            string
	accountName          string
	beneficiaryFirstName string
	beneficiaryLastName  string
	beneficiaryPhone     string
}

// loadForExecution locks the payout, validates its status and loads the destination.
// The transaction ends before any gateway call: short locks, no network under a lock.
func (s *Service) loadForExecution(ctx context.Context, id string, allowed ...string) (Payout, destination, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payout{}, destination{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := lock(ctx, tx, id, "")
	if err != nil {
		return Payout{}, destination{}, err
	}
	ok := false
	for _, st := range allowed {
		if p.Status == st {
			ok = true
		}
	}
	if !ok {
		return Payout{}, destination{}, ErrState
	}
	var d destination
	var enc []byte
	if err := tx.QueryRow(ctx, `
		SELECT b.bank_code, b.account_name, b.account_enc, COALESCE(m.contact_phone, '')
		FROM merchant_bank_accounts b JOIN merchants m ON m.id = b.merchant_id
		WHERE b.id = $1`, p.BankAccountID).Scan(&d.bankCode, &d.accountName, &enc, &d.beneficiaryPhone); err != nil {
		return Payout{}, destination{}, err
	}
	num, err := s.box.Open(enc, []byte(p.BankAccountID))
	if err != nil {
		return Payout{}, destination{}, err
	}
	d.accountNo = string(num)
	d.beneficiaryFirstName, d.beneficiaryLastName = splitName(d.accountName)
	return p.Payout, d, nil
}

// claimSubmit records approved -> submitted along with the gateway and the fee, in one
// atomic UPDATE: two concurrent Executes cannot both claim the same payout.
func (s *Service) claimSubmit(ctx context.Context, id, gwCode string, fee int64, a Actor) (Payout, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Payout{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := scanPayout(tx.QueryRow(ctx, `
		UPDATE payouts SET status = 'submitted', gateway = $2, fee_amount = $3, updated_at = now(), version = version + 1
		WHERE id = $1 AND status = 'approved' RETURNING `+payoutCols, id, gwCode, fee))
	if errors.Is(err, ErrNotFound) {
		return Payout{}, ErrState
	}
	if err != nil {
		return Payout{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.execute", EntityType: "payout", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"merchant_id": p.MerchantID, "amount": p.Amount, "fee": fee, "gateway": gwCode}}); err != nil {
		return Payout{}, err
	}
	if err := notify(ctx, tx, p, "submitted"); err != nil {
		return Payout{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payout{}, err
	}
	return p, nil
}

// storeGatewayReference records the reference the gateway returned for the transfer.
func (s *Service) storeGatewayReference(ctx context.Context, id, ref string, p Payout, a Actor) error {
	if ref == "" {
		return nil // nothing to record (e.g. a gateway that does not return one yet)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `UPDATE payouts SET bank_reference = $2 WHERE id = $1`, id, ref); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.gateway_reference", EntityType: "payout", EntityID: id,
		Metadata: map[string]any{"merchant_id": p.MerchantID, "gateway_reference": ref}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// markUnknown records an ambiguous outcome: the payout stays resolvable and the
// reservation is never released (PRD §0 #3); repeated check-status calls resolve it.
func (s *Service) markUnknown(ctx context.Context, id, reason string, a Actor) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx,
		`UPDATE payouts SET status = 'unknown', failure_reason = $2, updated_at = now(), version = version + 1
		 WHERE id = $1 AND status IN ('submitted','unknown')`, id, truncate(reason, 500)); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.unknown", EntityType: "payout", EntityID: id, RequestID: a.RequestID,
		Metadata: map[string]any{"reason": truncate(reason, 500)}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) auditCheckStatus(ctx context.Context, id string, a Actor, outcome string, extra map[string]any) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	md := map[string]any{"outcome": outcome}
	for k, v := range extra {
		md[k] = v
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: a.UserID, Action: "payout.check_status", EntityType: "payout", EntityID: id, RequestID: a.RequestID,
		Metadata: md}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// nameMismatch reports whether the gateway's name for the account differs from the name
// the merchant registered: the transfer must not go to a differently-owned account.
func nameMismatch(registered, reported string) string {
	if reported == "" || norm(registered) == norm(reported) {
		return ""
	}
	return fmt.Sprintf("bank account name mismatch: gateway reports %q, registered %q", reported, registered)
}

func norm(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(s)))
}

// splitName splits a full name into the first token and the rest, as the gateway's
// first/last name fields require.
func splitName(full string) (string, string) {
	f := strings.Fields(strings.TrimSpace(full))
	if len(f) == 0 {
		return "", ""
	}
	if len(f) == 1 {
		return f[0], ""
	}
	return f[0], strings.Join(f[1:], " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func refOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
