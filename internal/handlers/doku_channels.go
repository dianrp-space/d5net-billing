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

// dokuFeeFromBaseMDR: fee_flat = biaya dasar MDR (mis. Rp4.000 dari DOKU);
// fee_percent = % dari biaya dasar yang dibebankan ke customer.
// Persen kosong/0 = 100% (customer bayar penuh biaya dasar).
func dokuFeeFromBaseMDR(flat int64, percent float64) int64 {
	if flat <= 0 {
		return 0
	}
	share := percent
	if share <= 0 {
		share = 100
	}
	fee := int64(math.Round(float64(flat) * share / 100))
	if fee < 0 {
		return 0
	}
	return fee
}

// dokuChannelFeeConfig mengembalikan fee_flat & fee_percent efektif satu channel
// Direct (dipakai untuk menampilkan biaya di opsi bayar portal).
func dokuChannelFeeConfig(cfg dokuIntegrationStored, channelID string) (int64, float64) {
	fees := effectiveDokuChannels(cfg)
	f, ok := fees[strings.ToLower(strings.TrimSpace(channelID))]
	if !ok {
		return 0, 0
	}
	return f.FeeFlat, f.FeePercent
}

// dokuCustomerFeeForChannel menghitung biaya admin untuk satu channel Direct.
// invoiceBase hanya gate (tagihan harus > 0); persen dihitung dari fee_flat (MDR), bukan dari nominal invoice.
func dokuCustomerFeeForChannel(cfg dokuIntegrationStored, channelID string, invoiceBase int64) int64 {
	if normalizeDokuFeeMode(cfg.FeeMode) != DokuFeeModeCustomer || invoiceBase <= 0 {
		return 0
	}
	fees := effectiveDokuChannels(cfg)
	f, ok := fees[strings.ToLower(strings.TrimSpace(channelID))]
	if !ok || !f.Enabled {
		return 0
	}
	return dokuFeeFromBaseMDR(f.FeeFlat, f.FeePercent)
}
