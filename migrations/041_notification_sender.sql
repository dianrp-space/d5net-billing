-- +goose Up
-- Identitas pengirim aktual notifikasi (nomor/device WA, bot Telegram, From email)
-- untuk kolom Pengirim di riwayat + penelusuran kiriman gagal.
ALTER TABLE notification_queue ADD COLUMN IF NOT EXISTS sender TEXT;

-- +goose Down
ALTER TABLE notification_queue DROP COLUMN IF EXISTS sender;
