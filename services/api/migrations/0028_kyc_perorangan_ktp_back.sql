-- +goose Up
-- Jenis entitas "Perorangan" (tanpa dokumen badan usaha) + KTP sisi belakang.
ALTER TABLE merchants DROP CONSTRAINT IF EXISTS merchants_entity_type_check;
ALTER TABLE merchants ADD CONSTRAINT merchants_entity_type_check
    CHECK (entity_type IN ('PT','CV','Yayasan','Perorangan'));

ALTER TABLE merchant_documents DROP CONSTRAINT IF EXISTS merchant_documents_doc_type_check;
ALTER TABLE merchant_documents ADD CONSTRAINT merchant_documents_doc_type_check
    CHECK (doc_type IN ('ktp','ktp_back','npwp_badan','akta','sk_kemenkumham','nib','izin_usaha'));

-- +goose Down
DELETE FROM merchant_documents WHERE doc_type = 'ktp_back';
UPDATE merchants SET entity_type = NULL WHERE entity_type = 'Perorangan';
ALTER TABLE merchant_documents DROP CONSTRAINT IF EXISTS merchant_documents_doc_type_check;
ALTER TABLE merchant_documents ADD CONSTRAINT merchant_documents_doc_type_check
    CHECK (doc_type IN ('ktp','npwp_badan','akta','sk_kemenkumham','nib','izin_usaha'));
ALTER TABLE merchants DROP CONSTRAINT IF EXISTS merchants_entity_type_check;
ALTER TABLE merchants ADD CONSTRAINT merchants_entity_type_check
    CHECK (entity_type IN ('PT','CV','Yayasan'));
