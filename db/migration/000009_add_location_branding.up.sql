ALTER TABLE locations
  ADD COLUMN logo_url text,
  ADD COLUMN primary_color varchar(7),
  ADD COLUMN accent_color varchar(7),
  ADD COLUMN background_color varchar(7),
  ADD COLUMN font_family varchar(64);
