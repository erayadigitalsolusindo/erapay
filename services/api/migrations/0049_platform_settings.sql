-- +goose Up
-- Setelan platform bernilai integer (satuan terkecil), diubah DEWA lewat panel dan dicatat di audit_logs.
-- qrisgate_fee: potongan per transaksi QRISGate sukses dari saldo tenant. Bawaan 0 = tidak memotong.
CREATE TABLE platform_settings (
    key        text PRIMARY KEY CHECK (key IN ('qrisgate_fee')),
    int_value  bigint NOT NULL CHECK (int_value >= 0),
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO platform_settings (key, int_value) VALUES ('qrisgate_fee', 0);

-- +goose Down
DROP TABLE platform_settings;
