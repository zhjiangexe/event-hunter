-- +goose Up
ALTER TABLE payments ADD COLUMN IF NOT EXISTS payment_id VARCHAR(200);
UPDATE payments SET payment_id = 'LEGACY-' || id::text WHERE payment_id IS NULL;
ALTER TABLE payments ALTER COLUMN payment_id SET NOT NULL;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'payments_payment_id_uq') THEN
        ALTER TABLE payments ADD CONSTRAINT payments_payment_id_uq UNIQUE (payment_id);
    END IF;
END $$;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check CHECK (status IN ('COMPLETED', 'FAILED', 'REFUNDED', 'VOIDED'));

-- +goose Down
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_status_check;
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check CHECK (status IN ('COMPLETED', 'REFUNDED', 'VOIDED'));
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_payment_id_uq;
ALTER TABLE payments DROP COLUMN IF EXISTS payment_id;
