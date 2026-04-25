CREATE TABLE "locations" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "owner_username" varchar NOT NULL REFERENCES "users" ("username") ON DELETE RESTRICT,
  "name" varchar NOT NULL,
  "slug" varchar NOT NULL,
  "timezone" varchar NOT NULL,
  "is_active" bool NOT NULL DEFAULT true,
  "booking_requires_host_approval" bool NOT NULL DEFAULT false,
  "cancellation_min_hours_before_start" int,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  "deleted_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  CONSTRAINT "locations_slug_lowercase" CHECK ("slug" = lower("slug")),
  CONSTRAINT "locations_cancellation_hours_nonneg" CHECK (
    "cancellation_min_hours_before_start" IS NULL OR "cancellation_min_hours_before_start" >= 0
  )
);

CREATE UNIQUE INDEX "locations_slug_active_uidx"
  ON "locations" ("slug")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE INDEX "locations_owner_username_idx"
  ON "locations" ("owner_username")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE TABLE "services" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "location_id" uuid NOT NULL REFERENCES "locations" ("id") ON DELETE RESTRICT,
  "name" varchar NOT NULL,
  "description" varchar NOT NULL DEFAULT '',
  "duration_minutes" int NOT NULL,
  "price_minor_units" bigint NOT NULL DEFAULT 0,
  "currency" varchar NOT NULL DEFAULT 'GBP',
  "is_active" bool NOT NULL DEFAULT true,
  "cancellation_min_hours_before_start" int,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  "deleted_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  CONSTRAINT "services_duration_positive" CHECK ("duration_minutes" > 0),
  CONSTRAINT "services_cancellation_hours_nonneg" CHECK (
    "cancellation_min_hours_before_start" IS NULL OR "cancellation_min_hours_before_start" >= 0
  )
);

CREATE INDEX "services_location_id_idx"
  ON "services" ("location_id")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE TABLE "practitioners" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "location_id" uuid NOT NULL REFERENCES "locations" ("id") ON DELETE RESTRICT,
  "name" varchar NOT NULL,
  "display_name" varchar NOT NULL,
  "is_active" bool NOT NULL DEFAULT true,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  "deleted_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z'
);

CREATE INDEX "practitioners_location_id_idx"
  ON "practitioners" ("location_id")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE TABLE "rooms" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "location_id" uuid NOT NULL REFERENCES "locations" ("id") ON DELETE RESTRICT,
  "name" varchar NOT NULL,
  "is_active" bool NOT NULL DEFAULT true,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  "deleted_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z'
);

CREATE INDEX "rooms_location_id_idx"
  ON "rooms" ("location_id")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE TABLE "appointment_slots" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "location_id" uuid NOT NULL REFERENCES "locations" ("id") ON DELETE RESTRICT,
  "service_id" uuid NOT NULL REFERENCES "services" ("id") ON DELETE RESTRICT,
  "practitioner_id" uuid REFERENCES "practitioners" ("id") ON DELETE RESTRICT,
  "room_id" uuid REFERENCES "rooms" ("id") ON DELETE RESTRICT,
  "start_at" timestamptz NOT NULL,
  "end_at" timestamptz NOT NULL,
  "capacity" int NOT NULL DEFAULT 1,
  "booked_count" int NOT NULL DEFAULT 0,
  "status" varchar NOT NULL DEFAULT 'available',
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  "deleted_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  CONSTRAINT "appointment_slots_capacity_positive" CHECK ("capacity" > 0),
  CONSTRAINT "appointment_slots_booked_nonneg" CHECK ("booked_count" >= 0),
  CONSTRAINT "appointment_slots_booked_lte_capacity" CHECK ("booked_count" <= "capacity"),
  CONSTRAINT "appointment_slots_time_order" CHECK ("end_at" > "start_at"),
  CONSTRAINT "appointment_slots_status_values" CHECK (
    "status" IN ('available', 'booked', 'cancelled', 'unavailable')
  )
);

CREATE INDEX "appointment_slots_location_start_idx"
  ON "appointment_slots" ("location_id", "start_at")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE TABLE "bookings" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "location_id" uuid NOT NULL REFERENCES "locations" ("id") ON DELETE RESTRICT,
  "slot_id" uuid NOT NULL REFERENCES "appointment_slots" ("id") ON DELETE RESTRICT,
  "status" varchar NOT NULL,
  "guest_name" varchar NOT NULL,
  "guest_email" varchar NOT NULL,
  "guest_phone" varchar,
  "guest_notes" varchar,
  "client_username" varchar REFERENCES "users" ("username") ON DELETE SET NULL,
  "booked_at" timestamptz NOT NULL DEFAULT (now()),
  "cancelled_at" timestamptz,
  "cancel_reason" varchar,
  "cancel_token_hash" varchar(64) NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "updated_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  "deleted_at" timestamptz NOT NULL DEFAULT '0001-01-01 00:00:00Z',
  CONSTRAINT "bookings_status_values" CHECK (
    "status" IN ('pending', 'confirmed', 'cancelled', 'completed', 'no_show')
  )
);

CREATE INDEX "bookings_slot_id_idx"
  ON "bookings" ("slot_id")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE INDEX "bookings_location_id_idx"
  ON "bookings" ("location_id")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz;

CREATE INDEX "bookings_client_username_idx"
  ON "bookings" ("client_username")
  WHERE "deleted_at" = '0001-01-01 00:00:00Z'::timestamptz AND "client_username" IS NOT NULL;
