package payment

import "strings"

// DOKU Direct channel kinds surfaced to the portal.
const (
	DokuKindQR      = "qr"
	DokuKindVA      = "va"
	DokuKindEwallet = "ewallet"
	DokuKindRetail  = "retail"
)

// DokuChannel describes one payment method selectable in our portal (not Checkout).
type DokuChannel struct {
	ID         string // app id, e.g. "va_bca"
	Label      string
	Kind       string // qr | va | ewallet | retail
	SnapCode   string // DOKU additionalInfo.channel (VA / e-wallet)
	DefaultOn  bool
	NeedsSNAP  bool // private key + merchant/terminal/postal for QRIS
	NeedsVABin bool // partnerServiceId / BIN from DOKU BO
}

// DokuChannelCatalog is the fixed list of Direct channels we support.
func DokuChannelCatalog() []DokuChannel {
	return []DokuChannel{
		{ID: "qris", Label: "QRIS", Kind: DokuKindQR, DefaultOn: true, NeedsSNAP: true},

		{ID: "va_bca", Label: "VA BCA", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BCA", DefaultOn: true, NeedsVABin: true},
		{ID: "va_bri", Label: "VA BRI", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BRI", DefaultOn: true, NeedsVABin: true},
		{ID: "va_bni", Label: "VA BNI", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BNI", DefaultOn: true, NeedsVABin: true},
		{ID: "va_mandiri", Label: "VA Mandiri", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BANK_MANDIRI", DefaultOn: true, NeedsVABin: true},
		{ID: "va_cimb", Label: "VA CIMB", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BANK_CIMB", DefaultOn: false, NeedsVABin: true},
		{ID: "va_permata", Label: "VA Permata", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_PERMATA", DefaultOn: false, NeedsVABin: true},
		{ID: "va_danamon", Label: "VA Danamon", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BANK_DANAMON", DefaultOn: false, NeedsVABin: true},
		{ID: "va_bsi", Label: "VA BSI", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BSI", DefaultOn: false, NeedsVABin: true},
		{ID: "va_btn", Label: "VA BTN", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BTN", DefaultOn: false, NeedsVABin: true},
		{ID: "va_maybank", Label: "VA Maybank", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_MAYBANK", DefaultOn: false, NeedsVABin: true},
		{ID: "va_bnc", Label: "VA BNC", Kind: DokuKindVA, SnapCode: "VIRTUAL_ACCOUNT_BNC", DefaultOn: false, NeedsVABin: true},

		{ID: "ewallet_dana", Label: "DANA", Kind: DokuKindEwallet, SnapCode: "EMONEY_DANA_SNAP", DefaultOn: true},
		{ID: "ewallet_shopeepay", Label: "ShopeePay", Kind: DokuKindEwallet, SnapCode: "EMONEY_SHOPEEPAY_SNAP", DefaultOn: true},
		{ID: "ewallet_ovo", Label: "OVO", Kind: DokuKindEwallet, SnapCode: "EMONEY_OVO_SNAP", DefaultOn: false},

		{ID: "retail_alfamart", Label: "Alfamart / Alfa Group", Kind: DokuKindRetail, DefaultOn: true},
		{ID: "retail_indomaret", Label: "Indomaret", Kind: DokuKindRetail, DefaultOn: true},
	}
}

// LookupDokuChannel returns a catalog entry by app channel id.
func LookupDokuChannel(id string) (DokuChannel, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, c := range DokuChannelCatalog() {
		if c.ID == id {
			return c, true
		}
	}
	return DokuChannel{}, false
}

// PadPartnerServiceID left-pads BIN to 8 chars with spaces (SNAP VA requirement).
func PadPartnerServiceID(bin string) string {
	s := strings.TrimSpace(bin)
	if len(s) >= 8 {
		return s[:8]
	}
	return strings.Repeat(" ", 8-len(s)) + s
}
