package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/bri"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/doku"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/fake"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/finpay"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/ids"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/midtrans"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/xendit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gwcred"
)

// gatewayFactories is where gateway codes meet concrete adapter packages (PRD §0 #11). A new
// gateway adds its credential schema in gwcred and its factory here.
func gatewayFactories() map[string]gwcred.Factory {
	return map[string]gwcred.Factory{doku.Code: dokuFromCreds, ids.Code: idsFromCreds, finpay.Code: finpayFromCreds, xendit.Code: xenditFromCreds, midtrans.Code: midtransFromCreds}
}

// idsFromCreds builds the IDS adapter from credentials saved in the panel. IDS keys carry no
// sandbox/production marker we can rely on, so the environment is not cross-checked here.
func idsFromCreds(c map[string]string, _ string) (gateway.Adapter, error) {
	return ids.New(ids.Config{BaseURL: c["base_url"], APIKey: c["api_key"], CallbackSecret: c["callback_secret"]})
}

// xenditFromCreds builds the Xendit adapter. Xendit has one host, so the environment is enforced on
// the key prefix: a test key can never serve production (or the reverse) by a copy-paste slip.
func xenditFromCreds(c map[string]string, env string) (gateway.Adapter, error) {
	want := "xnd_development_"
	if env == "production" {
		want = "xnd_production_"
	}
	if !strings.HasPrefix(c["secret_key"], want) {
		return nil, errors.New("xendit: secret key does not match the environment")
	}
	return xendit.New(xendit.Config{SecretKey: c["secret_key"]})
}

// midtransFromCreds builds the Midtrans adapter. The host follows the environment and the server key
// must carry the matching SB- marker, so a sandbox key never serves production (or the reverse).
// The signature key has no marker we can rely on (the sandbox one is not SB-), so it is not cross-checked.
func midtransFromCreds(c map[string]string, env string) (gateway.Adapter, error) {
	base, sandboxKey := midtrans.SandboxURL, strings.HasPrefix(c["server_key"], "SB-")
	if env == "production" {
		base = midtrans.ProductionURL
	}
	if sandboxKey != (env != "production") {
		return nil, errors.New("midtrans: server key does not match the environment")
	}
	return midtrans.New(midtrans.Config{BaseURL: base, ServerKey: c["server_key"], SignatureKey: c["signature_key"]})
}

// finpayFromCreds builds the FINPAY adapter. The host follows the environment, so sandbox keys can
// never reach production (or the reverse) by a copy-paste slip.
func finpayFromCreds(c map[string]string, env string) (gateway.Adapter, error) {
	base := finpay.SandboxURL
	if env == "production" {
		base = finpay.ProductionURL
	}
	return finpay.New(finpay.Config{BaseURL: base, MerchantID: c["merchant_id"], MerchantKey: c["merchant_key"], CallbackURL: c["callback_url"]})
}

// dokuFromCreds builds the DOKU adapter from credentials saved in the panel. Unlike the env path
// it insists the base URL matches the environment, so sandbox keys never hit production (or the
// reverse) by a copy-paste slip.
func dokuFromCreds(c map[string]string, env string) (gateway.Adapter, error) {
	key, err := doku.ParsePrivateKey([]byte(c["private_key"]))
	if err != nil {
		return nil, err
	}
	base := c["base_url"]
	switch {
	case env == "production" && base == "":
		base = doku.ProductionURL
	case env == "production" && base != doku.ProductionURL:
		return nil, errors.New("base URL sandbox tidak boleh dipakai di environment production")
	case env == "sandbox" && base != "" && base != doku.SandboxBaseURL:
		return nil, errors.New("base URL production tidak boleh dipakai di environment sandbox")
	}
	return doku.New(doku.Config{BaseURL: base, ClientID: c["client_id"], ClientSecret: c["secret_key"], PrivateKey: key,
		TerminalID: c["terminal_id"], PostalCode: c["postal_code"], StatusMapConfirmed: strings.EqualFold(c["status_map_confirmed"], "true")})
}

// buildRegistry is the ONLY place a concrete gateway package is imported (PRD §0 #11).
// Credentials saved in the panel (creds, may be nil) win over DOKU_* env; when none are saved,
// or they fail to load, the environment is used exactly as before.
func buildRegistry(cfg config.Config, log *slog.Logger, creds *gwcred.Service) (*gateway.Registry, error) {
	reg := gateway.NewRegistry()
	if err := registerDOKU(reg, cfg, log, creds); err != nil {
		return nil, err
	}
	registerFromPanel(reg, cfg, log, creds, ids.Code, idsFromCreds)
	registerFromPanel(reg, cfg, log, creds, finpay.Code, finpayFromCreds)
	registerFromPanel(reg, cfg, log, creds, xendit.Code, xenditFromCreds)
	registerFromPanel(reg, cfg, log, creds, midtrans.Code, midtransFromCreds)
	registerBRI(reg, cfg, log)
	return reg, nil
}

// buildSandboxRegistry holds the adapters sandbox merchants use on a production server. Only credentials
// saved for the sandbox environment feed it (no env fallback), so production keys can never end up here.
func buildSandboxRegistry(log *slog.Logger, creds *gwcred.Service, fakeGW bool) *gateway.Registry {
	reg := gateway.NewRegistry()
	if fakeGW {
		reg.Register(fake.New()) // sandbox only: it has no upstream and never exists in the production registry
		log.Info("gateway registered", "gateway", fake.Code, "env", "sandbox")
	}
	if creds == nil {
		return reg
	}
	for code, build := range gatewayFactories() {
		vals, ok, err := creds.Load(context.Background(), code, "sandbox")
		if err != nil {
			log.Error("sandbox gateway credentials unreadable", "gateway", code)
			continue
		}
		if !ok {
			continue
		}
		a, err := build(vals, "sandbox")
		if err != nil {
			log.Error("sandbox gateway credentials rejected", "gateway", code, "reason", err.Error())
			continue
		}
		reg.Register(a)
		log.Info("gateway registered from database credentials", "gateway", code, "env", "sandbox")
	}
	return reg
}

// registerFromPanel registers a gateway (IDS, FINPAY) only from credentials saved in the panel (there is no env fallback):
// without them the gateway simply does not exist, and a merchant account pointing at it reports
// PAYMENT_METHOD_UNAVAILABLE. Unreadable or rejected credentials are logged, never fatal, so a
// broken setup cannot stop DOKU from serving.
func registerFromPanel(reg *gateway.Registry, cfg config.Config, log *slog.Logger, creds *gwcred.Service, code string, build gwcred.Factory) {
	if creds == nil {
		return
	}
	env := apikey.EnvFromApp(cfg.Env)
	vals, ok, err := creds.Load(context.Background(), code, env)
	switch {
	case err != nil:
		log.Error("gateway credentials in database unreadable", "gateway", code)
		return
	case !ok:
		return
	}
	a, err := build(vals, env)
	if err != nil {
		log.Error("gateway credentials in database rejected", "gateway", code, "reason", err.Error())
		return
	}
	reg.Register(a)
	log.Info("gateway registered from database credentials", "gateway", code, "env", env)
}

// registerBRI registers BRI from BRI_* env (the panel credential form comes later). Like IDS it is
// never fatal: missing or rejected settings are logged and BRI simply does not exist, so a
// half-filled BRI setup cannot stop the other gateways from serving.
func registerBRI(reg *gateway.Registry, cfg config.Config, log *slog.Logger) {
	bc, err := config.LoadBRI()
	if err != nil {
		log.Error("BRI configuration rejected", "reason", err.Error())
		return
	}
	if bc == nil {
		return
	}
	key, err := bri.ParsePrivateKey(bc.PrivateKeyPEM)
	if err != nil {
		log.Error("BRI configuration rejected", "reason", err.Error())
		return
	}
	base := bc.BaseURL
	if base == "" && cfg.Env == "production" {
		log.Error("BRI configuration rejected", "reason", "BRI_BASE_URL (production) is required when ERAPAY_ENV=production")
		return
	}
	a, err := bri.New(bri.Config{BaseURL: base, ClientID: bc.ClientID, ClientSecret: bc.ClientSecret, PrivateKey: key,
		PartnerID: bc.PartnerID, ChannelID: bc.ChannelID, MerchantID: bc.MerchantID, TerminalID: bc.TerminalID})
	if err != nil {
		log.Error("BRI configuration rejected", "reason", err.Error())
		return
	}
	reg.Register(a)
	log.Info("gateway registered", "gateway", bri.Code)
}

func registerDOKU(reg *gateway.Registry, cfg config.Config, log *slog.Logger, creds *gwcred.Service) error {
	env := apikey.EnvFromApp(cfg.Env)
	if creds != nil {
		vals, ok, err := creds.Load(context.Background(), doku.Code, env)
		switch {
		case err != nil:
			log.Error("gateway credentials in database unreadable; falling back to env", "gateway", doku.Code)
		case ok:
			a, err := dokuFromCreds(vals, env)
			if err != nil {
				log.Error("gateway credentials in database rejected; falling back to env", "gateway", doku.Code, "reason", err.Error())
				break
			}
			reg.Register(a)
			log.Info("gateway registered from database credentials", "gateway", doku.Code, "env", env)
			return nil
		}
	}
	dc, err := config.LoadDOKU()
	if err != nil {
		return err
	}
	if dc == nil {
		log.Warn("DOKU not configured: payments will report PAYMENT_METHOD_UNAVAILABLE")
		return nil
	}
	key, err := doku.ParsePrivateKey(dc.PrivateKeyPEM)
	if err != nil {
		return err
	}
	base := dc.BaseURL
	if base == "" && cfg.Env == "production" {
		base = doku.ProductionURL
	}
	a, err := doku.New(doku.Config{BaseURL: base, ClientID: dc.ClientID, ClientSecret: dc.ClientSecret,
		PrivateKey: key, TerminalID: dc.TerminalID, PostalCode: dc.PostalCode, StatusMapConfirmed: dc.StatusMapConfirmed})
	if err != nil {
		return err
	}
	reg.Register(a)
	log.Info("gateway registered", "gateway", doku.Code, "status_map_confirmed", dc.StatusMapConfirmed)
	return nil
}

// registryDisburser lets payouts follow the adapter currently registered, so rotating credentials
// from the panel also reaches payouts without a restart.
type registryDisburser struct{ reg *gateway.Registry }

var errNoDisburser = errors.New("gateway: no adapter registered for payouts")

func (r registryDisburser) cur() (gateway.Disburser, error) {
	if a, ok := r.reg.Get(doku.Code); ok {
		if d, ok := a.(gateway.Disburser); ok {
			return d, nil
		}
	}
	return nil, errNoDisburser
}

func (r registryDisburser) Code() string { return doku.Code }

func (r registryDisburser) InquiryBankAccount(ctx context.Context, q gateway.BankInquiryRequest) (gateway.BankInquiryResult, error) {
	d, err := r.cur()
	if err != nil {
		return gateway.BankInquiryResult{}, err
	}
	return d.InquiryBankAccount(ctx, q)
}

func (r registryDisburser) TransferBank(ctx context.Context, q gateway.TransferRequest) (gateway.TransferResult, error) {
	d, err := r.cur()
	if err != nil {
		return gateway.TransferResult{}, err
	}
	return d.TransferBank(ctx, q)
}

func (r registryDisburser) TransferStatus(ctx context.Context, q gateway.TransferStatusRequest) (gateway.TransferStatusResult, error) {
	d, err := r.cur()
	if err != nil {
		return gateway.TransferStatusResult{}, err
	}
	return d.TransferStatus(ctx, q)
}

func (r registryDisburser) MapTransferStatus(raw string) (gateway.TransferStatus, bool) {
	d, err := r.cur()
	if err != nil {
		return "", false
	}
	return d.MapTransferStatus(raw)
}

// disburserOf returns a payout executor that tracks the registered adapter, or false when the
// registered one cannot execute payouts. Adding a second gateway means adding its code here
// (the domain stays gateway-neutral, PRD §0 #11).
func disburserOf(reg *gateway.Registry) (gateway.Disburser, bool) {
	d := registryDisburser{reg: reg}
	if _, err := d.cur(); err != nil {
		return nil, false
	}
	return d, true
}
