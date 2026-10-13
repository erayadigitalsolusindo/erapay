-- +goose Up
CREATE TABLE merchant_external_links (
    source              text NOT NULL CHECK (source = 'arus'),
    source_tenant_id    uuid NOT NULL,
    merchant_id         uuid NOT NULL UNIQUE REFERENCES merchants(id),
    owner_user_id       uuid NOT NULL REFERENCES users(id),
    terms_accepted_at   timestamptz NOT NULL,
    terms_accepted_by   text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, source_tenant_id)
);

-- +goose Down
DROP TABLE merchant_external_links;