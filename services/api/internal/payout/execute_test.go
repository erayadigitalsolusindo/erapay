package payout

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// fakeDisburser is a TEST-ONLY executor gateway: it answers the disbursement contract
// with scripted outcomes and counts the calls so tests can prove exactly how many times
// money was asked to move.
type fakeDisburser struct {
	code        string
	inquiry     gateway.BankInquiryResult
	inquiryErr  error
	transfer    gateway.TransferResult
	transferErr error
	status      gateway.TransferStatusResult
	statusErr   error

	inquiries  atomic.Int32
	transfers  atomic.Int32
	checks     atomic.Int32
	lastAmount int64
	lastRef    string
}

func (f *fakeDisburser) Code() string { return f.code }

func (f *fakeDisburser) InquiryBankAccount(context.Context, gateway.BankInquiryRequest) (gateway.BankInquiryResult, error) {
	f.inquiries.Add(1)
	return f.inquiry, f.inquiryErr
}

func (f *fakeDisburser) TransferBank(_ context.Context, r gateway.TransferRequest) (gateway.TransferResult, error) {
	f.transfers.Add(1)
	f.lastAmount = r.Amount
	f.lastRef = r.Reference
	return f.transfer, f.transferErr
}

func (f *fakeDisburser) TransferStatus(context.Context, gateway.TransferStatusRequest) (gateway.TransferStatusResult, error) {
	f.checks.Add(1)
	return f.status, f.statusErr
}

func (f *fakeDisburser) MapTransferStatus(raw string) (gateway.TransferStatus, bool) {
	switch raw {
	case "00":
		return gateway.TransferSucceeded, true
	case "03":
		return gateway.TransferPending, true
	case "06":
		return gateway.TransferFailed, true
	case "04":
		return gateway.TransferRefunded, true
	}
	return "", false
}

func execSetup(t *testing.T, mods ...func(*Config)) *env {
	t.Helper()
	e := setup(t, append([]func(*Config){func(c *Config) {
		c.DryRun = false
		c.LiveEnabled = true
		c.FeeFlat = 1500
		c.Sender = Sender{CustomerNumber: "6281234567890", FirstName: "PT EraPay", LastName: "Digital",
			PersonalID: "0123456789012345", IDType: "KTP"}
	}}, mods...)...)
	if _, err := e.db.Exec(context.Background(), `UPDATE merchants SET contact_phone = '6281234567890'`); err != nil {
		t.Fatal(err)
	}
	return e
}

// approvedPayout funds the merchant, then requests, verifies and approves one payout.
func approvedPayout(t *testing.T, e *env, amount int64) Payout {
	t.Helper()
	e.n++
	e.fund(t, e.a, amount+10000) // covers fees so the request stays below available
	acct := e.account(t, e.a, e.ua)
	p, _, err := e.req(e.a, e.ua, acct, amount, fmt.Sprintf("idem-key-%d", e.n))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Approve(context.Background(), p.ID, Actor{UserID: e.fin, RequestID: "t"}); err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *env) leg(t *testing.T, code, dir string) int64 {
	t.Helper()
	return e.count(t, `SELECT COALESCE(SUM(le.amount),0) FROM ledger_entries le JOIN ledger_accounts a ON a.id = le.account_id
		WHERE a.code = $1 AND le.direction = $2`, code, dir)
}

func (e *env) inPayout(t *testing.T, m string) int64 {
	t.Helper()
	return e.bal(t, m).InPayout
}

func TestExecuteSubmitsAndCheckStatusPays(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 100_000)
	f := &fakeDisburser{code: "doku",
		inquiry:  gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT", FeeAmount: 1500},
		transfer: gateway.TransferResult{GatewayReference: "KD-1"}}
	e.svc.SetDisburser(f)

	if e.inPayout(t, e.a) != 100_000 {
		t.Fatalf("in_payout = %d, want reservation", e.inPayout(t, e.a))
	}
	out, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin, RequestID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "submitted" || out.Gateway != "doku" || out.Fee != 1500 || out.BankReference == nil || *out.BankReference != "KD-1" {
		t.Fatalf("%+v", out)
	}
	if f.transfers.Load() != 1 || f.lastAmount != 98_500 || f.lastRef != p.ID {
		t.Fatalf("transfer called %d times, amount=%d ref=%q", f.transfers.Load(), f.lastAmount, f.lastRef)
	}
	if f.inquiries.Load() != 1 {
		t.Fatalf("inquiry called %d times", f.inquiries.Load())
	}
	// The beneficiary gets the nominal minus the fee; the fee is its own ledger leg.
	f.status = gateway.TransferStatusResult{RawStatus: "00"}
	out, err = e.svc.CheckStatus(context.Background(), p.ID, Actor{UserID: e.fin, RequestID: "t"})
	if err != nil || out.Status != "paid" || out.PaidAt == nil {
		t.Fatalf("paid: %v err=%v", out, err)
	}
	if e.leg(t, "payout_in_transit", "debit") != 100_000 ||
		e.leg(t, "cash_bank_pt", "credit") != 98_500 ||
		e.leg(t, "payout_fee_payable", "credit") != 1500 {
		t.Fatalf("paid journal legs: transit=%d cash=%d fee=%d",
			e.leg(t, "payout_in_transit", "debit"), e.leg(t, "cash_bank_pt", "credit"), e.leg(t, "payout_fee_payable", "credit"))
	}
	if e.inPayout(t, e.a) != 0 {
		t.Fatalf("in_payout after paid = %d, want 0", e.inPayout(t, e.a))
	}
}

func TestExecuteIsIdempotentPerPayout(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry:  gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT"},
		transfer: gateway.TransferResult{GatewayReference: "KD-1"}}
	e.svc.SetDisburser(f)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("second execute: %v, want ErrState", err)
	}
	if f.transfers.Load() != 1 {
		t.Fatalf("transfer called %d times, want 1", f.transfers.Load())
	}
}

// The financial invariant: many Finances clicking at once must move the money once.
func TestConcurrentExecuteTransfersOnce(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry:  gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT"},
		transfer: gateway.TransferResult{GatewayReference: "KD-1"}}
	e.svc.SetDisburser(f)
	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin, RequestID: "t"}); err == nil {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || f.transfers.Load() != 1 {
		t.Fatalf("executed %d times, transferred %d times: want exactly 1/1", ok.Load(), f.transfers.Load())
	}
}

func TestExecuteDefinitiveRejectionFailsAndReleases(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry:     gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT"},
		transferErr: &gateway.Error{Kind: gateway.Rejected, Message: "beneficiary bank offline"}}
	e.svc.SetDisburser(f)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !gateway.IsRejected(err) {
		t.Fatalf("err=%v", err)
	}
	out, err := e.svc.GetAny(context.Background(), p.ID)
	if err != nil || out.Status != "failed" {
		t.Fatalf("status=%v err=%v", out, err)
	}
	if e.inPayout(t, e.a) != 0 {
		t.Fatalf("rejection must release the reservation, in_payout=%d", e.inPayout(t, e.a))
	}
	if f.transfers.Load() != 1 {
		t.Fatalf("transfer must be attempted once, got %d", f.transfers.Load())
	}
}

func TestExecuteAmbiguousIsUnknownAndNeverReleased(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry:     gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT"},
		transferErr: &gateway.Error{Kind: gateway.Ambiguous, Message: "no response from gateway"}}
	e.svc.SetDisburser(f)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !gateway.IsAmbiguous(err) {
		t.Fatalf("err=%v", err)
	}
	out, _ := e.svc.GetAny(context.Background(), p.ID)
	if out.Status != "unknown" {
		t.Fatalf("status=%s, want unknown", out.Status)
	}
	if e.inPayout(t, e.a) != 50_000 {
		t.Fatalf("ambiguous outcome must hold the reservation, in_payout=%d", e.inPayout(t, e.a))
	}
	// Retrying does not double-send: unknown is not approved.
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("second execute: %v, want ErrState", err)
	}
	// Pending check status changes nothing; a later failure resolves and releases.
	f.status = gateway.TransferStatusResult{RawStatus: "03"}
	if _, err := e.svc.CheckStatus(context.Background(), p.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	if out, _ := e.svc.GetAny(context.Background(), p.ID); out.Status != "unknown" || e.inPayout(t, e.a) != 50_000 {
		t.Fatalf("pending must not change anything: %+v in_payout=%d", out, e.inPayout(t, e.a))
	}
	f.status = gateway.TransferStatusResult{RawStatus: "06"}
	if _, err := e.svc.CheckStatus(context.Background(), p.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	if out, _ := e.svc.GetAny(context.Background(), p.ID); out.Status != "failed" || e.inPayout(t, e.a) != 0 {
		t.Fatalf("failed must release: %+v in_payout=%d", out, e.inPayout(t, e.a))
	}
}

func TestCheckStatusPaidFromUnknownRecordsTraceableReference(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry:     gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT", FeeAmount: 1500},
		transferErr: &gateway.Error{Kind: gateway.Ambiguous, Message: "timeout"}}
	e.svc.SetDisburser(f)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !gateway.IsAmbiguous(err) {
		t.Fatal(err)
	}
	f.status = gateway.TransferStatusResult{RawStatus: "00"}
	if _, err := e.svc.CheckStatus(context.Background(), p.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	out, _ := e.svc.GetAny(context.Background(), p.ID)
	if out.Status != "paid" || out.BankReference == nil || *out.BankReference != "doku:"+p.ID {
		t.Fatalf("%+v", out)
	}
	if e.leg(t, "payout_fee_payable", "credit") != 1500 {
		t.Fatalf("fee leg missing after late resolution")
	}
}

func TestCheckStatusReturnedReleases(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry:  gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT"},
		transfer: gateway.TransferResult{GatewayReference: "KD-1"}}
	e.svc.SetDisburser(f)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	f.status = gateway.TransferStatusResult{RawStatus: "04"}
	if _, err := e.svc.CheckStatus(context.Background(), p.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	if out, _ := e.svc.GetAny(context.Background(), p.ID); out.Status != "returned" || e.inPayout(t, e.a) != 0 {
		t.Fatalf("returned must release: %+v in_payout=%d", out, e.inPayout(t, e.a))
	}
}

func TestExecuteNameMismatchFails(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	f := &fakeDisburser{code: "doku",
		inquiry: gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "Somebody Else"}}
	e.svc.SetDisburser(f)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrNameMismatch) {
		t.Fatalf("err=%v", err)
	}
	if out, _ := e.svc.GetAny(context.Background(), p.ID); out.Status != "failed" || e.inPayout(t, e.a) != 0 {
		t.Fatalf("mismatch must fail and release: %+v in_payout=%d", out, e.inPayout(t, e.a))
	}
	if f.transfers.Load() != 0 {
		t.Fatal("no transfer may be sent on a name mismatch")
	}
}

func TestExecuteDryRunSkipsGateway(t *testing.T) {
	e := execSetup(t, func(c *Config) { c.DryRun = true; c.LiveEnabled = false })
	p := approvedPayout(t, e, 100_000)
	e.svc.SetDisburser(&fakeDisburser{code: "doku"})
	out, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin})
	if err != nil || out.Status != "submitted" || out.Gateway != "dry-run" || out.Fee != 1500 || out.BankReference != nil {
		t.Fatalf("%+v err=%v", out, err)
	}
	// Finance confirms the dry-run transfer manually, the fee leg included.
	if _, err := e.svc.MarkPaid(context.Background(), p.ID, "MANUAL-REF", Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	if e.leg(t, "payout_fee_payable", "credit") != 1500 || e.leg(t, "cash_bank_pt", "credit") != 98_500 {
		t.Fatalf("dry-run paid legs: cash=%d fee=%d", e.leg(t, "cash_bank_pt", "credit"), e.leg(t, "payout_fee_payable", "credit"))
	}
	// A dry-run payout has no gateway transfer to check; keep it manual.
	if _, err := e.svc.CheckStatus(context.Background(), p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("check status on dry-run: %v, want ErrState", err)
	}
}

func TestExecuteRequiresDisburserAndSender(t *testing.T) {
	e := execSetup(t)
	p := approvedPayout(t, e, 50_000)
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrNoDisburser) {
		t.Fatalf("err=%v, want ErrNoDisburser", err)
	}
	// Connected gateway but no sender identity: nothing leaves the system.
	e.svc.SetDisburser(&fakeDisburser{code: "doku",
		inquiry: gateway.BankInquiryResult{SessionID: "INQ-1", AccountName: "PT MERCHANT"}})
	e.svc.cfg.Sender = Sender{}
	if _, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrValidation) {
		t.Fatalf("err=%v, want ErrValidation", err)
	}
	if out, _ := e.svc.GetAny(context.Background(), p.ID); out.Status != "approved" {
		t.Fatalf("a bad submit must leave the payout untouched: %s", out.Status)
	}
}
