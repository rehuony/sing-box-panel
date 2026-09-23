CREATE TABLE traffic_months (
    month_start TEXT PRIMARY KEY CHECK (month_start <> ''),
    inbound_bytes INTEGER NOT NULL CHECK (inbound_bytes >= 0),
    outbound_bytes INTEGER NOT NULL CHECK (outbound_bytes >= 0),
    has_delta INTEGER NOT NULL CHECK (has_delta IN (0, 1)),
    complete INTEGER NOT NULL CHECK (complete IN (0, 1)),
    first_observed_at TEXT NOT NULL CHECK (first_observed_at <> ''),
    last_sample_at TEXT NOT NULL CHECK (last_sample_at >= first_observed_at)
) STRICT;
