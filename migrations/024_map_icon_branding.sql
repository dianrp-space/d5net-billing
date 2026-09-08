-- +goose Up
-- +goose StatementBegin

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS map_pop_icon_url TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS map_odp_icon_url TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS map_customer_icon_url TEXT;

ALTER TABLE platform_branding ADD COLUMN IF NOT EXISTS map_pop_icon_url TEXT;
ALTER TABLE platform_branding ADD COLUMN IF NOT EXISTS map_odp_icon_url TEXT;
ALTER TABLE platform_branding ADD COLUMN IF NOT EXISTS map_customer_icon_url TEXT;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE platform_branding DROP COLUMN IF EXISTS map_pop_icon_url;
ALTER TABLE platform_branding DROP COLUMN IF EXISTS map_odp_icon_url;
ALTER TABLE platform_branding DROP COLUMN IF EXISTS map_customer_icon_url;

ALTER TABLE tenants DROP COLUMN IF EXISTS map_pop_icon_url;
ALTER TABLE tenants DROP COLUMN IF EXISTS map_odp_icon_url;
ALTER TABLE tenants DROP COLUMN IF EXISTS map_customer_icon_url;
-- +goose StatementEnd
