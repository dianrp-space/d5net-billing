-- +goose Up
-- +goose StatementBegin

-- Per 2026-09-22 seluruh pipeline trafik memakai perspektif PELANGGAN
-- (rx = download, tx = upload). Sebelumnya nilai mentah RouterOS
-- (perspektif router: rx = upload user) disimpan apa adanya, sehingga histori
-- yang sudah terkumpul tertukar. Tukar kolom histori agar konsisten.
-- traffic_last_sample ikut ditukar agar delta sampling berikutnya tetap
-- benar (sampel lama & baru sama-sama perspektif pelanggan).
UPDATE traffic_monthly SET rx_bytes = tx_bytes, tx_bytes = rx_bytes;
UPDATE traffic_last_sample SET rx_bytes = tx_bytes, tx_bytes = rx_bytes;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE traffic_monthly SET rx_bytes = tx_bytes, tx_bytes = rx_bytes;
UPDATE traffic_last_sample SET rx_bytes = tx_bytes, tx_bytes = rx_bytes;

-- +goose StatementEnd
