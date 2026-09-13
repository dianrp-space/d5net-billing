package payment

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

const (
	DokuVACreatePath   = "/virtual-accounts/bi-snap-va/v1.1/transfer-va/create-va"
	DokuEwalletPayPath = "/direct-debit/core/v1/debit/payment-host-to-host"
	DokuAlfaPath       = "/alfa-virtual-account/v2/payment-code"
	DokuIndomaretPath  = "/indomaret-online-to-offline/v2/payment-code"
)

func (p *DokuProvider) createQRIntent(ctx context.Context, req IntentRequest) (*IntentResult, error) {
	orderID := dokuOrderID(req)
	qr, err := p.GenerateQR(ctx, orderID, req.Amount)
	if err != nil {
		return nil, err
	}
	png, err := qrcode.Encode(qr.Content, qrcode.Medium, 512)
	if err != nil {
		return nil, fmt.Errorf("DOKU QRIS: encode image: %w", err)
	}
	return &IntentResult{
		ExternalID:    orderID,
		TransactionID: qr.Reference,
		QRString:      qr.Content,
		QRImageBase64: base64.StdEncoding.EncodeToString(png),
		Status:        "pending",
		ExpiresAt:     qr.ExpiresAt,
		Amount:        req.Amount,
		PayableAmount: req.Amount,
		Metadata: map[string]any{
			"doku_sandbox": p.Sandbox,
			"doku_kind":    DokuKindQR,
			"doku_channel": "qris",
			"reference":    qr.Reference,
		},
	}, nil
}

func (p *DokuProvider) createVAIntent(ctx context.Context, req IntentRequest, ch DokuChannel) (*IntentResult, error) {
	if strings.TrimSpace(p.PartnerServiceID) == "" {
		return nil, fmt.Errorf("BIN VA DOKU (partner service ID) belum diisi di Integrasi")
	}
	orderID := dokuOrderID(req)
	name := strings.TrimSpace(req.CustomerName)
	if name == "" {
		name = "Pelanggan"
	}
	expiry := ClampDokuExpiryMinutes(p.ExpiresInMinutes)
	expired := time.Now().Add(time.Duration(expiry) * time.Minute).In(dokuWIB).Format("2006-01-02T15:04:05+07:00")
	partnerSvc := PadPartnerServiceID(p.PartnerServiceID)
	// DGPC: kirim customerNo unik; DOKU menggabungkan BIN + customerNo jadi nomor VA.
	customerNo := dokuVACustomerNo(orderID)
	vaNo := partnerSvc + customerNo
	m, err := dokuSnapDo(ctx, p, DokuVACreatePath, map[string]any{
		"partnerServiceId":      partnerSvc,
		"customerNo":            customerNo,
		"virtualAccountNo":      vaNo,
		"virtualAccountName":    truncateRunes(name, 255),
		"virtualAccountEmail":   firstNonEmpty(strings.TrimSpace(req.Email), "noreply@localhost"),
		"virtualAccountPhone":   dokuPhone(req.Phone),
		"trxId":                 truncateRunes(orderID, 64),
		"totalAmount":           map[string]any{"value": fmt.Sprintf("%d.00", req.Amount), "currency": "IDR"},
		"virtualAccountTrxType": "C",
		"expiredDate":           expired,
		"additionalInfo": map[string]any{
			"channel": ch.SnapCode,
			"virtualAccountConfig": map[string]any{
				"reusableStatus": false,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	vaData, _ := m["virtualAccountData"].(map[string]any)
	gotVA := firstString(vaData, "virtualAccountNo")
	if gotVA == "" {
		gotVA = firstString(m, "virtualAccountNo")
	}
	if gotVA == "" {
		gotVA = strings.TrimSpace(vaNo)
	}
	exp := time.Now().Add(time.Duration(expiry) * time.Minute)
	return &IntentResult{
		ExternalID:    orderID,
		TransactionID: firstString(vaData, "inquiryRequestId", "trxId"),
		Status:        "pending",
		ExpiresAt:     &exp,
		Amount:        req.Amount,
		PayableAmount: req.Amount,
		Metadata: map[string]any{
			"doku_sandbox":       p.Sandbox,
			"doku_kind":          DokuKindVA,
			"doku_channel":       ch.ID,
			"doku_snap_channel":  ch.SnapCode,
			"va_number":          gotVA,
			"va_bank":            ch.Label,
			"partner_service_id": partnerSvc,
			"customer_no":        customerNo,
		},
	}, nil
}

func (p *DokuProvider) createEwalletIntent(ctx context.Context, req IntentRequest, ch DokuChannel) (*IntentResult, error) {
	orderID := dokuOrderID(req)
	returnURL := firstNonEmpty(strings.TrimSpace(req.ReturnURL), strings.TrimSpace(req.CallbackURL))
	if returnURL == "" {
		return nil, fmt.Errorf("return URL wajib untuk e-wallet DOKU")
	}
	expiry := ClampDokuExpiryMinutes(p.ExpiresInMinutes)
	validUpTo := time.Now().Add(time.Duration(expiry) * time.Minute).In(dokuWIB).Format("2006-01-02T15:04:05+07:00")
	m, err := dokuSnapDo(ctx, p, DokuEwalletPayPath, map[string]any{
		"partnerReferenceNo": orderID,
		"validUpTo":          validUpTo,
		"pointOfInitiation":  "mweb",
		"amount":             map[string]any{"value": fmt.Sprintf("%d.00", req.Amount), "currency": "IDR"},
		"urlParam": map[string]any{
			"url":        returnURL,
			"type":       "PAY_RETURN",
			"isDeepLink": "N",
		},
		"additionalInfo": map[string]any{
			"channel":                    ch.SnapCode,
			"orderTitle":                 truncateRunes(firstNonEmpty(req.ProductDetails, "Tagihan "+orderID), 64),
			"supportDeepLinkCheckoutUrl": "true",
		},
	})
	if err != nil {
		return nil, err
	}
	redirect := firstString(m, "webRedirectUrl", "redirectUrl", "checkoutUrl")
	if redirect == "" {
		return nil, fmt.Errorf("DOKU e-wallet: redirect URL kosong (%v)", m["responseMessage"])
	}
	exp := time.Now().Add(time.Duration(expiry) * time.Minute)
	return &IntentResult{
		ExternalID:    orderID,
		TransactionID: firstString(m, "referenceNo"),
		CheckoutURL:   redirect,
		Status:        "pending",
		ExpiresAt:     &exp,
		Amount:        req.Amount,
		PayableAmount: req.Amount,
		Metadata: map[string]any{
			"doku_sandbox":      p.Sandbox,
			"doku_kind":         DokuKindEwallet,
			"doku_channel":      ch.ID,
			"doku_snap_channel": ch.SnapCode,
		},
	}, nil
}

func dokuOrderID(req IntentRequest) string {
	orderID := strings.TrimSpace(req.MerchantOrderID)
	if orderID == "" {
		orderID = fmt.Sprintf("inv-%s", req.InvoiceID)
	}
	return truncateRunes(orderID, 64)
}

// dokuVACustomerNo builds a numeric customerNo (max ~20) from the order id.
func dokuVACustomerNo(orderID string) string {
	var b strings.Builder
	for _, r := range orderID {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		s = fmt.Sprintf("%d", time.Now().Unix()%1_000_000_000)
	}
	if len(s) > 12 {
		s = s[len(s)-12:]
	}
	return s
}
