ALTER TABLE locations
  DROP COLUMN IF EXISTS logo_url,
  DROP COLUMN IF EXISTS primary_color,
  DROP COLUMN IF EXISTS accent_color,
  DROP COLUMN IF EXISTS background_color,
  DROP COLUMN IF EXISTS font_family;
