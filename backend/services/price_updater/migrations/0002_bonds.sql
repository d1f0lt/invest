-- Bonds support. MOEX quotes bonds in % of face value (price_snapshots
-- keeps the quote exactly as MOEX publishes it); these two columns let
-- consumers turn a quote into money per bond: quote * face_value / 100.
ALTER TABLE securities ADD COLUMN IF NOT EXISTS face_value NUMERIC;
ALTER TABLE securities ADD COLUMN IF NOT EXISTS price_in_percent BOOLEAN NOT NULL DEFAULT false;
