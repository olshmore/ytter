-- name: GetLocationBySlug :one
SELECT * FROM locations
WHERE slug = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: GetLocationByID :one
SELECT * FROM locations
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: GetBookingByID :one
SELECT * FROM bookings
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: ListPublicSlotsByLocationSlug :many
SELECT
  s.id AS slot_id,
  s.start_at,
  s.end_at,
  s.status AS slot_status,
  s.capacity,
  s.booked_count,
  sv.id AS service_id,
  sv.name AS service_name,
  sv.duration_minutes,
  sv.price_minor_units,
  sv.currency,
  COALESCE(
    sv.cancellation_min_hours_before_start,
    l.cancellation_min_hours_before_start,
    24
  )::int AS effective_cancellation_min_hours_before_start,
  p.id AS practitioner_id,
  p.display_name AS practitioner_display_name,
  r.id AS room_id,
  r.name AS room_name,
  l.id AS location_id,
  l.slug AS location_slug,
  l.name AS location_name,
  l.timezone AS location_timezone,
  l.booking_requires_host_approval
FROM locations l
JOIN appointment_slots s ON s.location_id = l.id
JOIN services sv ON sv.id = s.service_id
LEFT JOIN practitioners p ON p.id = s.practitioner_id
LEFT JOIN rooms r ON r.id = s.room_id
WHERE l.slug = $1
  AND l.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND l.is_active = true
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.is_active = true
ORDER BY s.start_at ASC;

-- name: ListPublicFilterServicesByLocationSlug :many
SELECT sv.id, sv.name
FROM services sv
JOIN locations l ON l.id = sv.location_id
WHERE l.slug = $1
  AND l.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.is_active = true
ORDER BY sv.name ASC;

-- name: ListPublicFilterPractitionersByLocationSlug :many
SELECT p.id, p.display_name
FROM practitioners p
JOIN locations l ON l.id = p.location_id
WHERE l.slug = $1
  AND l.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND p.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND p.is_active = true
ORDER BY p.display_name ASC;

-- name: ListPublicFilterRoomsByLocationSlug :many
SELECT r.id, r.name
FROM rooms r
JOIN locations l ON l.id = r.location_id
WHERE l.slug = $1
  AND l.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND r.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND r.is_active = true
ORDER BY r.name ASC;

-- name: GetSlotForUpdate :one
SELECT * FROM appointment_slots
WHERE id = $1
  AND location_id = $2
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
FOR UPDATE;

-- name: CreateBooking :one
INSERT INTO bookings (
  location_id,
  slot_id,
  status,
  guest_name,
  guest_email,
  guest_phone,
  guest_notes,
  cancel_token_hash,
  client_username
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: UpdateSlotCounters :one
UPDATE appointment_slots
SET
  booked_count = $2,
  status = $3,
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetBookingForCancelByIDForUpdate :one
SELECT
  b.*,
  s.start_at,
  s.booked_count,
  s.capacity,
  sv.cancellation_min_hours_before_start AS service_cancellation_min_hours_before_start,
  l.cancellation_min_hours_before_start AS location_cancellation_min_hours_before_start
FROM bookings b
JOIN appointment_slots s ON s.id = b.slot_id
JOIN services sv ON sv.id = s.service_id
JOIN locations l ON l.id = b.location_id
WHERE b.id = $1
  AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
FOR UPDATE;

-- name: MarkBookingCancelled :one
UPDATE bookings
SET
  status = 'cancelled',
  cancelled_at = now(),
  cancel_reason = $2,
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ConfirmBooking :one
UPDATE bookings
SET
  status = 'confirmed',
  updated_at = now()
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
RETURNING *;

-- name: GetBookingForHostOpByIDForUpdate :one
SELECT
  b.id,
  b.location_id,
  b.slot_id,
  b.status,
  b.guest_name,
  b.guest_email,
  b.guest_phone,
  b.guest_notes,
  b.client_username,
  b.booked_at,
  b.cancelled_at,
  b.cancel_reason,
  b.cancel_token_hash,
  b.created_at,
  b.updated_at,
  b.deleted_at,
  s.start_at,
  s.booked_count,
  s.capacity,
  l.owner_username
FROM bookings b
JOIN appointment_slots s ON s.id = b.slot_id
JOIN locations l ON l.id = b.location_id
WHERE b.id = $1
  AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
FOR UPDATE;

-- name: ListHostLocationsByOwner :many
SELECT
  id,
  owner_username,
  slug,
  name,
  timezone,
  is_active,
  booking_requires_host_approval,
  COALESCE(cancellation_min_hours_before_start, 24)::int AS effective_cancellation_min_hours_before_start
FROM locations
WHERE owner_username = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
ORDER BY name ASC;

-- name: ListAllHostLocations :many
SELECT
  id,
  owner_username,
  slug,
  name,
  timezone,
  is_active,
  booking_requires_host_approval,
  COALESCE(cancellation_min_hours_before_start, 24)::int AS effective_cancellation_min_hours_before_start
FROM locations
WHERE deleted_at = '0001-01-01 00:00:00Z'::timestamptz
ORDER BY name ASC;

-- name: CreateLocation :one
INSERT INTO locations (
  owner_username,
  name,
  slug,
  timezone,
  is_active
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING *;

-- name: UpdateLocation :one
UPDATE locations
SET
  name = COALESCE(sqlc.narg(name), name),
  slug = COALESCE(sqlc.narg(slug), slug),
  timezone = COALESCE(sqlc.narg(timezone), timezone),
  is_active = COALESCE(sqlc.narg(is_active), is_active),
  booking_requires_host_approval = COALESCE(sqlc.narg(booking_requires_host_approval), booking_requires_host_approval),
  updated_at = now()
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
RETURNING *;

-- name: CountHostBookingsByLocation :one
SELECT COUNT(*)::int4 AS count
FROM bookings b
JOIN appointment_slots s ON s.id = b.slot_id
JOIN locations l ON l.id = b.location_id
WHERE b.location_id = $1
  AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND (
    COALESCE(sqlc.narg(filter_status)::text, '') = ''
    OR b.status = sqlc.narg(filter_status)
  )
  AND (
    COALESCE(sqlc.narg(from_date)::text, '') = ''
    OR (s.start_at AT TIME ZONE l.timezone)::date >= sqlc.narg(from_date)::date
  )
  AND (
    COALESCE(sqlc.narg(to_date)::text, '') = ''
    OR (s.start_at AT TIME ZONE l.timezone)::date <= sqlc.narg(to_date)::date
  );

-- name: ListHostBookingsByLocation :many
SELECT
  b.id AS booking_id,
  b.status,
  b.booked_at,
  b.guest_name,
  b.guest_email,
  b.guest_phone,
  b.cancel_reason,
  s.id AS slot_id,
  sv.name AS service_name,
  COALESCE(p.display_name, '') AS practitioner_name,
  COALESCE(r.name, '') AS room_name,
  s.start_at,
  s.end_at
FROM bookings b
JOIN appointment_slots s ON s.id = b.slot_id
JOIN services sv ON sv.id = s.service_id
LEFT JOIN practitioners p ON p.id = s.practitioner_id
LEFT JOIN rooms r ON r.id = s.room_id
JOIN locations l ON l.id = b.location_id
WHERE b.location_id = $1
  AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND (
    COALESCE(sqlc.narg(filter_status)::text, '') = ''
    OR b.status = sqlc.narg(filter_status)
  )
  AND (
    COALESCE(sqlc.narg(from_date)::text, '') = ''
    OR (s.start_at AT TIME ZONE l.timezone)::date >= sqlc.narg(from_date)::date
  )
  AND (
    COALESCE(sqlc.narg(to_date)::text, '') = ''
    OR (s.start_at AT TIME ZONE l.timezone)::date <= sqlc.narg(to_date)::date
  )
ORDER BY s.start_at ASC, b.booked_at DESC
LIMIT $2
OFFSET $3;

-- name: GetServiceByID :one
SELECT * FROM services
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: GetHostServiceByID :one
SELECT * FROM services
WHERE id = $1
  AND location_id = $2
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: CreateHostLocationService :one
INSERT INTO services (
  location_id,
  name,
  description,
  duration_minutes,
  price_minor_units,
  currency,
  is_active,
  cancellation_min_hours_before_start
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: UpdateHostLocationService :one
UPDATE services
SET
  name = COALESCE(sqlc.narg(name), name),
  description = COALESCE(sqlc.narg(description), description),
  duration_minutes = COALESCE(sqlc.narg(duration_minutes), duration_minutes),
  price_minor_units = COALESCE(sqlc.narg(price_minor_units), price_minor_units),
  currency = COALESCE(sqlc.narg(currency), currency),
  is_active = COALESCE(sqlc.narg(is_active), is_active),
  cancellation_min_hours_before_start = COALESCE(sqlc.narg(cancellation_min_hours_before_start), cancellation_min_hours_before_start),
  updated_at = now()
WHERE id = $1
  AND location_id = $2
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
RETURNING *;

-- name: CountHostServicesByLocation :one
SELECT COUNT(*)::int4 AS count
FROM services s
WHERE s.location_id = $1
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND (
    sqlc.narg(filter_is_active)::bool IS NULL
    OR s.is_active = sqlc.narg(filter_is_active)
  );

-- name: ListHostServicesByLocation :many
SELECT
  id,
  location_id,
  name,
  description,
  duration_minutes,
  price_minor_units,
  currency,
  is_active,
  cancellation_min_hours_before_start
FROM services s
WHERE s.location_id = $1
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND (
    sqlc.narg(filter_is_active)::bool IS NULL
    OR s.is_active = sqlc.narg(filter_is_active)
  )
ORDER BY s.name ASC
LIMIT $2
OFFSET $3;

-- name: GetPractitionerByID :one
SELECT * FROM practitioners
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: GetRoomByID :one
SELECT * FROM rooms
WHERE id = $1
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: GetHostSlotByID :one
SELECT * FROM appointment_slots
WHERE id = $1
  AND location_id = $2
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: CreateHostLocationSlot :one
INSERT INTO appointment_slots (
  location_id,
  service_id,
  practitioner_id,
  room_id,
  start_at,
  end_at,
  capacity,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: UpdateHostLocationSlot :one
UPDATE appointment_slots
SET
  service_id = COALESCE(sqlc.narg(service_id), service_id),
  practitioner_id = COALESCE(sqlc.narg(practitioner_id), practitioner_id),
  room_id = COALESCE(sqlc.narg(room_id), room_id),
  start_at = COALESCE(sqlc.narg(start_at), start_at),
  end_at = COALESCE(sqlc.narg(end_at), end_at),
  capacity = COALESCE(sqlc.narg(capacity), capacity),
  status = COALESCE(sqlc.narg(status), status),
  updated_at = now()
WHERE id = $1
  AND location_id = $2
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
RETURNING *;

-- name: CountHostSlotsByLocation :one
SELECT COUNT(*)::int4 AS count
FROM appointment_slots s
WHERE s.location_id = $1
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND (
    COALESCE(sqlc.narg(filter_status)::text, '') = ''
    OR s.status = sqlc.narg(filter_status)
  )
  AND (
    COALESCE(sqlc.narg(from_date)::text, '') = ''
    OR s.start_at::date >= sqlc.narg(from_date)::date
  )
  AND (
    COALESCE(sqlc.narg(to_date)::text, '') = ''
    OR s.start_at::date <= sqlc.narg(to_date)::date
  )
  AND (
    COALESCE(sqlc.narg(filter_service_id)::uuid, NULL::uuid) IS NULL
    OR s.service_id = sqlc.narg(filter_service_id)
  )
  AND (
    COALESCE(sqlc.narg(filter_practitioner_id)::uuid, NULL::uuid) IS NULL
    OR s.practitioner_id = sqlc.narg(filter_practitioner_id)
  )
  AND (
    COALESCE(sqlc.narg(filter_room_id)::uuid, NULL::uuid) IS NULL
    OR s.room_id = sqlc.narg(filter_room_id)
  );

-- name: ListHostSlotsByLocation :many
SELECT
  s.id,
  s.location_id,
  s.service_id,
  sv.name AS service_name,
  s.practitioner_id,
  COALESCE(p.display_name, '') AS practitioner_name,
  s.room_id,
  COALESCE(r.name, '') AS room_name,
  s.start_at,
  s.end_at,
  s.capacity,
  s.booked_count,
  s.status
FROM appointment_slots s
JOIN services sv ON sv.id = s.service_id
LEFT JOIN practitioners p ON p.id = s.practitioner_id
LEFT JOIN rooms r ON r.id = s.room_id
WHERE s.location_id = $1
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND (
    COALESCE(sqlc.narg(filter_status)::text, '') = ''
    OR s.status = sqlc.narg(filter_status)
  )
  AND (
    COALESCE(sqlc.narg(from_date)::text, '') = ''
    OR s.start_at::date >= sqlc.narg(from_date)::date
  )
  AND (
    COALESCE(sqlc.narg(to_date)::text, '') = ''
    OR s.start_at::date <= sqlc.narg(to_date)::date
  )
  AND (
    COALESCE(sqlc.narg(filter_service_id)::uuid, NULL::uuid) IS NULL
    OR s.service_id = sqlc.narg(filter_service_id)
  )
  AND (
    COALESCE(sqlc.narg(filter_practitioner_id)::uuid, NULL::uuid) IS NULL
    OR s.practitioner_id = sqlc.narg(filter_practitioner_id)
  )
  AND (
    COALESCE(sqlc.narg(filter_room_id)::uuid, NULL::uuid) IS NULL
    OR s.room_id = sqlc.narg(filter_room_id)
  )
ORDER BY s.start_at ASC
LIMIT $2
OFFSET $3;
