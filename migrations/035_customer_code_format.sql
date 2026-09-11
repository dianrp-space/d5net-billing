-- +goose Up
-- Format kode pelanggan baru: {prefix}-{yyyymm}{seq} (mis. BTC-2026090001).
-- Update default kolom + baris yang masih memakai pola default lama.
ALTER TABLE sites ALTER COLUMN customer_code_pattern SET DEFAULT '{prefix}-{yyyymm}{seq}';

UPDATE sites
SET customer_code_pattern = '{prefix}-{yyyymm}{seq}'
WHERE customer_code_pattern = '{prefix}-{yyyymm}-{seq}';

-- +goose Down
ALTER TABLE sites ALTER COLUMN customer_code_pattern SET DEFAULT '{prefix}-{yyyymm}-{seq}';

UPDATE sites
SET customer_code_pattern = '{prefix}-{yyyymm}-{seq}'
WHERE customer_code_pattern = '{prefix}-{yyyymm}{seq}';
