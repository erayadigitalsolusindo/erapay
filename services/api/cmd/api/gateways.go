package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/doku"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/ids"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gwcred"
)

// gatewayFactories is where gateway codes meet concrete adapter packages (PRD §0 #11). A new
// gateway adds its credential schema in gwcred and its factory here.
func gatewayFactories() map[string]gwcred.Factory {
	return map[string]gwcred.Factory{doku.Code: dokuFromCreds, ids.Code: idsFromCreds}
}

// idsFromCreds builds the IDS adapter from credentials saved in the panel. IDS keys carry no
// sandbox/production marker we can rely on, so the environment is not cross-checked here.
func idsFromCreds(c map[string]string, _ string) (gateway.Adapter, error) {
	return ids.New(ids.Config{BaseURL: c["base_url"], APIKey: c["api_key"], CallbackSecret: c["callback_secret"]})
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
	registerIDS(reg, cfg, log, creds)
	return reg, nil
}

// registerIDS registers IDS only from credentials saved in the panel (there is no env fallback):
// without them the gateway simply does not exist, and a merchant account pointing at it reports
// PAYMENT_METHOD_UNAVAILABLE. Unreadable or rejected credentials are logged, never fatal, so a
// broken IDS setup cannot stop DOKU from serving.
func registerIDS(reg *gateway.Registry, cfg config.Config, log *slog.Logger, creds *gwcred.Service) {
	if creds == nil {
		return
	}
	env := apikey.EnvFromApp(cfg.Env)
	vals, ok, err := creds.Load(context.Background(), ids.Code, env)
	switch {
	case err != nil:
		log.Error("gateway credentials in database unreadable", "gateway", ids.Code)
		return
	case !ok:
		return
	}
	a, err := idsFromCreds(vals, env)
	if err != nil {
		log.Error("gateway credentials in database rejected", "gateway", ids.Code, "reason", err.Error())
		return
	}
	reg.Register(a)
	log.Info("gateway registered from database credentials", "gateway", ids.Code, "env", env)
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
