ALTER TABLE users ADD COLUMN role VARCHAR NOT NULL DEFAULT 'client';

UPDATE users
SET role = CASE
  WHEN 'admin' = ANY(roles) THEN 'admin'
  WHEN 'host' = ANY(roles) THEN 'member'
  WHEN 'client' = ANY(roles) THEN 'member'
  ELSE 'client'
END;

ALTER TABLE users DROP COLUMN roles;
