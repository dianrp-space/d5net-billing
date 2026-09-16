-- +goose Up
-- +goose StatementBegin

-- Opsi isolir per tagihan: hanya tagihan dengan isolir=true yang membuat
-- langganan diisolir saat lewat jatuh tempo + masa tenggang. Tagihan langganan
-- otomatis selalu true; tagihan manual default false (mis. pembelian CCTV).
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS isolir BOOLEAN NOT NULL DEFAULT TRUE;

-- Langganan sasaran isolir untuk tagihan manual dengan isolir=true. Tagihan
-- langganan otomatis tetap memakai subscription_id biasa.
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS isolir_subscription_id UUID REFERENCES subscriptions(id) ON DELETE SET NULL;

-- Penanda denda keterlambatan tagihan manual sudah ditambahkan (idempoten).
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS late_fee_applied_at TIMESTAMPTZ;

-- Pertahankan perilaku lama: invoice lama yang tidak terhubung langganan
-- (tagihan manual) tidak mengisolir.
UPDATE invoices SET isolir = (subscription_id IS NOT NULL);

-- Denda tagihan manual berlaku untuk tagihan yang lewat jatuh tempo setelah
-- upgrade ini. Tagihan manual lama yang sudah lewat jatuh tempo ditandai agar
-- tidak dikenai denda mundur (mencegah tagihan lama membengkak).
UPDATE invoices SET late_fee_applied_at = NOW()
WHERE subscription_id IS NULL
  AND deleted_at IS NULL
  AND status IN ('issued','partial','overdue')
  AND total_amount > paid_amount
  AND due_date < CURRENT_DATE
  AND late_fee_applied_at IS NULL;

CREATE INDEX IF NOT EXISTS invoices_isolir_sub_idx
  ON invoices (tenant_id, isolir_subscription_id)
  WHERE isolir_subscription_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS invoices_isolir_sub_idx;
ALTER TABLE invoices DROP COLUMN IF EXISTS late_fee_applied_at;
ALTER TABLE invoices DROP COLUMN IF EXISTS isolir_subscription_id;
ALTER TABLE invoices DROP COLUMN IF EXISTS isolir;
-- +goose StatementEnd
