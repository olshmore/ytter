CREATE TABLE waitlist_entries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id uuid NOT NULL REFERENCES locations(id),
  service_id uuid NOT NULL REFERENCES services(id),
  slot_id uuid NOT NULL REFERENCES appointment_slots(id),
  guest_name varchar NOT NULL,
  guest_email varchar NOT NULL,
  guest_phone varchar,
  practitioner_id uuid REFERENCES practitioners(id),
  preferred_date date,
  status varchar NOT NULL DEFAULT 'active',
  offer_token_hash varchar,
  offer_expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z'
);

CREATE INDEX idx_waitlist_entries_slot_status
  ON waitlist_entries (slot_id, status)
  WHERE deleted_at = '0001-01-01 00:00:00Z';

CREATE INDEX idx_waitlist_entries_identity
  ON waitlist_entries (location_id, service_id, slot_id, lower(guest_email))
  WHERE deleted_at = '0001-01-01 00:00:00Z';
