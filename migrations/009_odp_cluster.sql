-- +goose Up
-- +goose StatementBegin
ALTER TABLE odps
    ADD COLUMN IF NOT EXISTS cluster_id UUID REFERENCES sites(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_odps_cluster_id ON odps(cluster_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_odps_cluster_id;
ALTER TABLE odps DROP COLUMN IF EXISTS cluster_id;
-- +goose StatementEnd
