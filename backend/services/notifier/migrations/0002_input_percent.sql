-- The percent the user entered when the target was set as change_percent
-- (NULL when it was set as a price). Only for showing/editing the alert in
-- the app; checks always use target_price.
ALTER TABLE alerts ADD COLUMN IF NOT EXISTS input_percent NUMERIC;
