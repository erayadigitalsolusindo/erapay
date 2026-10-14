package main

import (
	"log/slog"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/config"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/doku"
)

// buildRegistry is the ONLY place a concrete gateway package is imported (PRD §0 #11).
func buildRegistry(cfg config.Config, log *slog.Logger) (*gateway.Registry, error) {
	reg := gateway.NewRegistry()
	dc, err := config.LoadDOKU()
	if err != nil {
		return nil, err
	}
	if dc == nil {
		log.Warn("DOKU not configured: payments will report PAYMENT_METHOD_UNAVAILABLE")
		return reg, nil
	}
	key, err := doku.ParsePrivateKey(dc.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	base := dc.BaseURL
	if base == "" && cfg.Env == "production" {
		base = doku.ProductionURL
	}
	a, err := doku.New(doku.Config{BaseURL: base, ClientID: dc.ClientID, ClientSecret: dc.ClientSecret,
		PrivateKey: key, TerminalID: dc.TerminalID, PostalCode: dc.PostalCode, StatusMapConfirmed: dc.StatusMapConfirmed})
	if err != nil {
		return nil, err
	}
	reg.Register(a)
	log.Info("gateway registered", "gateway", doku.Code, "status_map_confirmed", dc.StatusMapConfirmed)
	return reg, nil
}

// disburserOf returns the first registered adapter that can execute payouts. Adding a
// second gateway means adding its code here (the domain stays gateway-neutral, PRD §0 #11).
func disburserOf(reg *gateway.Registry) (gateway.Disburser, bool) {
	if a, ok := reg.Get(doku.Code); ok {
		if d, ok := a.(gateway.Disburser); ok {
			return d, true
		}
	}
	return nil, false
}
