package handlers

import (
	"math"
	"strings"

	"github.com/dianrp-space/d5net-billing/internal/payment"
)

type dokuChannelFeeStored struct {
	Enabled          bool    `json:"enabled"`
	FeeFlat          int64   `json:"fee_flat"`
	FeePercent       float64 `json:"fee_percent"`
	PartnerServiceID string  `json:"partner_service_id,omitempty"` // BIN VA per bank
}

func clampFeePercent(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func effectiveDokuChannels(cfg dokuIntegrationStored) map[string]dokuChannelFeeStored {
	out := make(map[string]dokuChannelFeeStored, len(payment.DokuChannelCatalog()))
	legacy := normalizeDokuFeeMode(cfg.FeeMode) == DokuFeeModeCustomer && (cfg.FeeFlat > 0 || cfg.FeePercent > 0)
	legacyBIN := strings.TrimSpace(cfg.PartnerServiceID)
	for _, c := range payment.DokuChannelCatalog() {
		fee := dokuChannelFeeStored{Enabled: c.DefaultOn}
		if cfg.Channels != nil {
			if stored, ok := cfg.Channels[c.ID]; ok {
				fee = stored
			} else if legacy {
				fee.FeeFlat = cfg.FeeFlat
				fee.FeePercent = cfg.FeePercent
			}
		} else if legacy {
			fee.FeeFlat = cfg.FeeFlat
			fee.FeePercent = cfg.FeePercent
		}
		if fee.FeeFlat < 0 {
			fee.FeeFlat = 0
		}
		fee.FeePercent = clampFeePercent(fee.FeePercent)
		fee.PartnerServiceID = strings.TrimSpace(fee.PartnerServiceID)
		// Legacy: satu BIN global dipakai semua VA yang belum punya BIN sendiri.
		if c.NeedsVABin && fee.PartnerServiceID == "" && legacyBIN != "" {
			fee.PartnerServiceID = legacyBIN
		}
		out[c.ID] = fee
	}
	return out
}

func dokuChannelViews(cfg dokuIntegrationStored) []dokuChannelFeeView {
	fees := effectiveDokuChannels(cfg)
	out := make([]dokuChannelFeeView, 0, len(payment.DokuChannelCatalog()))
	for _, c := range payment.DokuChannelCatalog() {
		f := fees[c.ID]
		out = append(out, dokuChannelFeeView{
			ID: c.ID, Label: c.Label, Kind: c.Kind,
			Enabled: f.Enabled, FeeFlat: f.FeeFlat, FeePercent: f.FeePercent,
			PartnerServiceID: f.PartnerServiceID,
		})
	}
	return out
}

// dokuVABinForChannel mengembalikan Company Code / BIN untuk satu channel VA.
func dokuVABinForChannel(cfg dokuIntegrationStored, channelID string) string {
	fees := effectiveDokuChannels(cfg)
	f, ok := fees[strings.ToLower(strings.TrimSpace(channelID))]
	if !ok {
		return strings.TrimSpace(cfg.PartnerServiceID)
	}
	if bin := strings.TrimSpace(f.PartnerServiceID); bin != "" {
		return bin
	}
	return strings.TrimSpace(cfg.PartnerServiceID)
}

// dokuCustomerFeeForChannel menghitung biaya admin untuk satu channel Direct.
func dokuCustomerFeeForChannel(cfg dokuIntegrationStored, channelID string, base int64) int64 {
	if normalizeDokuFeeMode(cfg.FeeMode) != DokuFeeModeCustomer || base <= 0 {
		return 0
	}
	fees := effectiveDokuChannels(cfg)
	f, ok := fees[strings.ToLower(strings.TrimSpace(channelID))]
	if !ok || !f.Enabled {
		return 0
	}
	fee := f.FeeFlat
	if f.FeePercent > 0 {
		fee += int64(math.Round(float64(base) * f.FeePercent / 100))
	}
	if fee < 0 {
		return 0
	}
	return fee
}
