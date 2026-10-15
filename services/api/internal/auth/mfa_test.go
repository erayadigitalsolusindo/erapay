package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
	"github.com/pquerna/otp/totp"
)

// A user who never enrolled has mfa_secret NULL; status must report "not enrolled", not fail.
func TestMFAStatusNotEnrolled(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	uid, err := s.CreateUser(context.Background(), "plain@erapay.test", pw)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.GetMFAEnrollmentStatus(context.Background(), uid)
	if err != nil || info.MFAEnrolled {
		t.Fatalf("status for non-enrolled user: %+v err=%v", info, err)
	}
}

func TestMFAEnrollmentAndLoginFlow(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()

	uid, err := s.CreateUser(ctx, "root@erapay.test", pw)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AddPlatformRole(t, db, uid, "super_admin")

	enroll, err := s.EnrollMFA(ctx, uid, "root@erapay.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(enroll.RecoveryCodes) == 0 {
		t.Fatal("no recovery codes")
	}

	code, err := totp.GenerateCode(enroll.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmMFAEnrollment(ctx, uid, code, "req-enroll"); err != nil {
		t.Fatal(err)
	}

	// Status reflects enrollment.
	info, err := s.GetMFAEnrollmentStatus(ctx, uid)
	if err != nil || !info.MFAEnrolled {
		t.Fatalf("status: %+v err=%v", info, err)
	}

	// Login now requires MFA.
	mfaErr, ok := func() (*MFARequiredError, bool) {
		_, _, err := login(s, "root@erapay.test", pw, "")
		var me *MFARequiredError
		if errors.As(err, &me) {
			return me, true
		}
		return nil, false
	}()
	_ = mfaErr
	if !ok {
		t.Fatalf("expected MFARequiredError, got %v", err)
	}

	// Invalid code must fail.
	if _, _, err := s.CompleteMFALogin(ctx, LoginInput{
		Email: "root@erapay.test", Password: pw, IP: "10.0.0.1",
	}, "000000", mfaErr.ChallengeID); !errors.Is(err, ErrMFAInvalidCode) {
		t.Fatalf("invalid code: got %v, want ErrMFAInvalidCode", err)
	}

	// Valid fresh code completes login.
	freshCode, err := totp.GenerateCode(enroll.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tok, p, err := s.CompleteMFALogin(ctx, LoginInput{
		Email: "root@erapay.test", Password: pw, IP: "10.0.0.1",
	}, freshCode, mfaErr.ChallengeID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Logout(ctx, p, "req") }()
	if p.MerchantID != "" || !p.Has("gateway:manage") {
		t.Fatalf("bad principal: %+v", p)
	}
	if _, err := s.Authenticate(ctx, tok); err != nil {
		t.Fatalf("session not valid: %v", err)
	}

	// Keep enrollment/recovery artifacts referenced.
	_ = enroll.RecoveryCodes
	_ = pw
}

func TestMFAInvalidPasswordDuringVerify(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db, DefaultConfig())
	ctx := context.Background()

	uid, _ := s.CreateUser(ctx, "root2@erapay.test", pw)
	testdb.AddPlatformRole(t, db, uid, "super_admin")

	enroll, err := s.EnrollMFA(ctx, uid, "root2@erapay.test")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(enroll.Secret, time.Now())
	if err := s.ConfirmMFAEnrollment(ctx, uid, code, "req"); err != nil {
		t.Fatal(err)
	}

	ch, ok := func() (*MFARequiredError, bool) {
		_, _, err := login(s, "root2@erapay.test", pw, "")
		var me *MFARequiredError
		if errors.As(err, &me) {
			return me, true
		}
		return nil, false
	}()
	if !ok {
		t.Fatal("expected MFA required")
	}
	freshCode, _ := totp.GenerateCode(enroll.Secret, time.Now())
	if _, _, err := s.CompleteMFALogin(ctx, LoginInput{
		Email: "root2@erapay.test", Password: "wrong password", IP: "10.0.0.1",
	}, freshCode, ch.ChallengeID); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("want ErrInvalidCredentials, got %v", err)
	}
}
