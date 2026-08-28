-- +goose Up
ALTER TABLE shipments DROP CONSTRAINT IF EXISTS shipments_status_check;
ALTER TABLE shipments
    ADD CONSTRAINT shipments_status_check CHECK (status IN ('CREATED', 'DISPATCHED', 'IN_TRANSIT', 'DELIVERED', 'RETURN_RECEIVED'));

CREATE TABLE IF NOT EXISTS returns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id VARCHAR(200) NOT NULL UNIQUE,
    order_id VARCHAR(200) NOT NULL UNIQUE,
    shipment_id VARCHAR(200) NOT NULL,
    correlation_id VARCHAR(200) NOT NULL UNIQUE,
    status VARCHAR(30) NOT NULL CHECK (status IN ('REQUESTED', 'RECEIVED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS returns;
ALTER TABLE shipments DROP CONSTRAINT IF EXISTS shipments_status_check;
ALTER TABLE shipments
    ADD CONSTRAINT shipments_status_check CHECK (status IN ('CREATED'));
