ALTER TABLE users ADD COLUMN roles VARCHAR[] NOT NULL DEFAULT '{}';

UPDATE users
SET roles = CASE
  WHEN role = 'admin' THEN ARRAY['admin']::VARCHAR[]
  WHEN role = 'member' THEN ARRAY['host', 'client']::VARCHAR[]
  ELSE ARRAY['client']::VARCHAR[]
END;

ALTER TABLE users DROP COLUMN role;
