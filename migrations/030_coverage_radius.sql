-- +goose Up
-- +goose StatementBegin
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS coverage_radius_km DOUBLE PRECISION;

ALTER TABLE odps
    ADD COLUMN IF NOT EXISTS coverage_radius_km DOUBLE PRECISION;

COMMENT ON COLUMN sites.coverage_radius_km IS
  'POP coverage radius in km for sales map check; NULL/0 = not set';
COMMENT ON COLUMN odps.coverage_radius_km IS
  'ODP coverage radius in km for sales map check; NULL/0 = not set';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sites DROP COLUMN IF EXISTS coverage_radius_km;
ALTER TABLE odps DROP COLUMN IF EXISTS coverage_radius_km;
-- +goose StatementEnd
