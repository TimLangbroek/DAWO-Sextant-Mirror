-- Every reveal of a device secret, kept.
--
-- device_secrets.revealed_by holds only the LAST reader, and a new escrow
-- (a reprovisioned device, a rotated administrator password) clears it. So a
-- second reveal, or a rotation, erased the record of the one before, while the
-- console and its designs said every reveal was audited. This table is the
-- record: the product inserts a row per reveal and never updates or deletes
-- one. revealed_by on device_secrets stays, as the device page's "last read".
CREATE TABLE IF NOT EXISTS device_secret_reveals (
    tenant      text        NOT NULL,
    tag         text        NOT NULL,
    kind        text        NOT NULL,
    revealed_by text        NOT NULL,
    revealed    timestamptz NOT NULL
);

-- The evidence bundle and the SIEM export read a period.
CREATE INDEX IF NOT EXISTS device_secret_reveals_time ON device_secret_reveals (tenant, revealed);

-- What survived until now: the last reveal of each secret still held.
INSERT INTO device_secret_reveals (tenant, tag, kind, revealed_by, revealed)
SELECT tenant, tag, kind, revealed_by, revealed
FROM device_secrets
WHERE revealed IS NOT NULL AND revealed_by <> '';
