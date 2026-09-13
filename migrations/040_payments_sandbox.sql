-- +goose Up
-- Penanda mode sandbox pada pembayaran PG (kolom Metode tampil "Sandbox · Duitku").
ALTER TABLE payments ADD COLUMN IF NOT EXISTS sandbox BOOLEAN NOT NULL DEFAULT FALSE;

-- Backfill dari metadata payment intent (key sandbox_sim / duitku_sandbox / doku_sandbox),
-- dicocokkan lewat reference (= external id) atau pasangan invoice + provider.
UPDATE payments p SET sandbox = true
WHERE EXISTS (
  SELECT 1 FROM payment_intents pi
  WHERE pi.tenant_id = p.tenant_id
    AND (
      (pi.external_id IS NOT NULL AND pi.external_id <> '' AND pi.external_id = p.reference)
      OR (p.invoice_id IS NOT NULL AND pi.invoice_id = p.invoice_id AND pi.provider = p.method)
    )
    AND (
      COALESCE(pi.metadata->>'sandbox_sim', 'false') = 'true'
      OR COALESCE(pi.metadata->>'duitku_sandbox', 'false') = 'true'
      OR COALESCE(pi.metadata->>'doku_sandbox', 'false') = 'true'
    )
);

-- +goose Down
ALTER TABLE payments DROP COLUMN IF EXISTS sandbox;
