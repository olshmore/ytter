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
  l.booking_requires_host_approval,
  l.logo_url AS location_logo_url,
  l.primary_color AS location_primary_color,
  l.accent_color AS location_accent_color,
  l.background_color AS location_background_color,
  l.font_family AS location_font_family
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

-- name: ListPublicLocations :many
SELECT DISTINCT
  l.id,
  l.slug,
  l.name,
  l.timezone,
  l.booking_requires_host_approval,
  l.logo_url,
  l.primary_color,
  l.accent_color,
  l.background_color,
  l.font_family
FROM locations l
JOIN appointment_slots s ON s.location_id = l.id
JOIN services sv ON sv.id = s.service_id
WHERE l.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND l.is_active = true
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND s.start_at > now()
  AND sv.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.is_active = true
ORDER BY l.name ASC;

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

-- name: MarkBookingNoShow :one
UPDATE bookings
SET
  status = 'no_show',
  updated_at = now()
WHERE id = $1
  AND status = 'confirmed'
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
RETURNING *;

-- name: ClearBookingNoShow :one
UPDATE bookings
SET
  status = 'confirmed',
  updated_at = now()
WHERE id = $1
  AND status = 'no_show'
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
  COALESCE(cancellation_min_hours_before_start, 24)::int AS effective_cancellation_min_hours_before_start,
  logo_url,
  primary_color,
  accent_color,
  background_color,
  font_family
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
  COALESCE(cancellation_min_hours_before_start, 24)::int AS effective_cancellation_min_hours_before_start,
  logo_url,
  primary_color,
  accent_color,
  background_color,
  font_family
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

-- name: UpdateLocationBranding :one
UPDATE locations
SET
  logo_url = CASE
    WHEN @reset_branding::bool THEN NULL
    WHEN @clear_logo_url::bool THEN NULL
    WHEN @set_logo_url::bool THEN sqlc.narg(logo_url)
    ELSE logo_url
  END,
  primary_color = CASE
    WHEN @reset_branding::bool THEN NULL
    WHEN @clear_primary_color::bool THEN NULL
    WHEN @set_primary_color::bool THEN sqlc.narg(primary_color)
    ELSE primary_color
  END,
  accent_color = CASE
    WHEN @reset_branding::bool THEN NULL
    WHEN @clear_accent_color::bool THEN NULL
    WHEN @set_accent_color::bool THEN sqlc.narg(accent_color)
    ELSE accent_color
  END,
  background_color = CASE
    WHEN @reset_branding::bool THEN NULL
    WHEN @clear_background_color::bool THEN NULL
    WHEN @set_background_color::bool THEN sqlc.narg(background_color)
    ELSE background_color
  END,
  font_family = CASE
    WHEN @reset_branding::bool THEN NULL
    WHEN @clear_font_family::bool THEN NULL
    WHEN @set_font_family::bool THEN sqlc.narg(font_family)
    ELSE font_family
  END,
  updated_at = now()
WHERE id = @id
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
RETURNING *;

-- name: CountHostBookingsByLocation :one
WITH unified AS (
  SELECT
    b.status,
    s.start_at,
    l.timezone
  FROM bookings b
  JOIN appointment_slots s ON s.id = b.slot_id
  JOIN locations l ON l.id = b.location_id
  WHERE b.location_id = $1
    AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz

  UNION ALL

  SELECT
    'pending'::varchar AS status,
    s.start_at,
    l.timezone
  FROM waitlist_entries w
  JOIN appointment_slots s ON s.id = w.slot_id
  JOIN locations l ON l.id = w.location_id
  WHERE w.location_id = $1
    AND w.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
    AND w.status = 'active'
)
SELECT COUNT(*)::int4 AS count
FROM unified u
WHERE (
    COALESCE(sqlc.narg(filter_status)::text, '') = ''
    OR u.status = sqlc.narg(filter_status)
  )
  AND (
    COALESCE(sqlc.narg(from_date)::text, '') = ''
    OR (u.start_at AT TIME ZONE u.timezone)::date >= sqlc.narg(from_date)::date
  )
  AND (
    COALESCE(sqlc.narg(to_date)::text, '') = ''
    OR (u.start_at AT TIME ZONE u.timezone)::date <= sqlc.narg(to_date)::date
  );

-- name: ListHostBookingsByLocation :many
WITH unified AS (
  SELECT
    b.id AS booking_id,
    b.status,
    b.booked_at,
    b.guest_name,
    b.guest_email,
    b.guest_phone,
    b.cancel_reason,
    l.id AS location_id,
    l.slug AS location_slug,
    l.name AS location_name,
    s.id AS slot_id,
    sv.name AS service_name,
    COALESCE(p.display_name, '') AS practitioner_name,
    COALESCE(r.name, '') AS room_name,
    s.start_at,
    s.end_at,
    FALSE AS is_waitlist
  FROM bookings b
  JOIN appointment_slots s ON s.id = b.slot_id
  JOIN services sv ON sv.id = s.service_id
  LEFT JOIN practitioners p ON p.id = s.practitioner_id
  LEFT JOIN rooms r ON r.id = s.room_id
  JOIN locations l ON l.id = b.location_id
  WHERE b.location_id = $1
    AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz

  UNION ALL

  SELECT
    w.id AS booking_id,
    'pending'::varchar AS status,
    w.created_at AS booked_at,
    w.guest_name,
    w.guest_email,
    w.guest_phone,
    NULL::varchar AS cancel_reason,
    l.id AS location_id,
    l.slug AS location_slug,
    l.name AS location_name,
    s.id AS slot_id,
    sv.name AS service_name,
    COALESCE(p.display_name, '') AS practitioner_name,
    COALESCE(r.name, '') AS room_name,
    s.start_at,
    s.end_at,
    TRUE AS is_waitlist
  FROM waitlist_entries w
  JOIN appointment_slots s ON s.id = w.slot_id
  JOIN services sv ON sv.id = w.service_id
  LEFT JOIN practitioners p ON p.id = COALESCE(w.practitioner_id, s.practitioner_id)
  LEFT JOIN rooms r ON r.id = s.room_id
  JOIN locations l ON l.id = w.location_id
  WHERE w.location_id = $1
    AND w.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
    AND w.status = 'active'
)
SELECT
  u.booking_id,
  u.status,
  u.booked_at,
  u.guest_name,
  u.guest_email,
  u.guest_phone,
  u.cancel_reason,
  u.location_id,
  u.location_slug,
  u.location_name,
  u.slot_id,
  u.service_name,
  u.practitioner_name,
  u.room_name,
  u.start_at,
  u.end_at,
  u.is_waitlist
FROM unified u
JOIN locations l ON l.id = u.location_id
WHERE (
    COALESCE(sqlc.narg(filter_status)::text, '') = ''
    OR u.status = sqlc.narg(filter_status)
  )
  AND (
    COALESCE(sqlc.narg(from_date)::text, '') = ''
    OR (u.start_at AT TIME ZONE l.timezone)::date >= sqlc.narg(from_date)::date
  )
  AND (
    COALESCE(sqlc.narg(to_date)::text, '') = ''
    OR (u.start_at AT TIME ZONE l.timezone)::date <= sqlc.narg(to_date)::date
  )
ORDER BY u.start_at ASC, u.booked_at DESC
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

-- name: GetRebookContextByBookingID :one
SELECT
  b.id AS booking_id,
  b.client_username,
  b.guest_email,
  l.id AS location_id,
  l.slug AS location_slug,
  l.name AS location_name,
  sv.id AS service_id,
  sv.name AS service_name,
  s.practitioner_id,
  l.is_active AS location_is_active,
  sv.is_active AS service_is_active
FROM bookings b
JOIN locations l ON l.id = b.location_id
JOIN appointment_slots s ON s.id = b.slot_id
JOIN services sv ON sv.id = s.service_id
WHERE b.id = $1
  AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND l.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
  AND sv.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: CountMyBookings :one
SELECT COUNT(*)::int4 AS count
FROM bookings b
JOIN appointment_slots s ON s.id = b.slot_id
JOIN locations l ON l.id = b.location_id
WHERE (
    b.client_username = $1
    OR (
    COALESCE(sqlc.narg(filter_guest_email)::text, '') <> ''
    AND lower(b.guest_email) = lower(sqlc.narg(filter_guest_email))
    )
  )
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

-- name: ListMyBookings :many
SELECT
  b.id AS booking_id,
  b.status,
  b.booked_at,
  b.guest_name,
  b.guest_email,
  b.guest_phone,
  b.cancel_reason,
  l.id AS location_id,
  l.slug AS location_slug,
  l.name AS location_name,
  s.id AS slot_id,
  sv.name AS service_name,
  COALESCE(p.display_name, '') AS practitioner_name,
  COALESCE(r.name, '') AS room_name,
  s.start_at,
  s.end_at,
  EXISTS (
    SELECT 1
    FROM waitlist_entries w
    WHERE w.location_id = b.location_id
      AND w.service_id = s.service_id
      AND lower(w.guest_email) = lower(b.guest_email)
  ) AS is_waitlist
FROM bookings b
JOIN appointment_slots s ON s.id = b.slot_id
JOIN services sv ON sv.id = s.service_id
LEFT JOIN practitioners p ON p.id = s.practitioner_id
LEFT JOIN rooms r ON r.id = s.room_id
JOIN locations l ON l.id = b.location_id
WHERE (
    b.client_username = $1
    OR (
    COALESCE(sqlc.narg(filter_guest_email)::text, '') <> ''
    AND lower(b.guest_email) = lower(sqlc.narg(filter_guest_email))
    )
  )
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

-- name: GetActiveWaitlistEntryByIdentity :one
SELECT *
FROM waitlist_entries
WHERE location_id = $1
  AND service_id = $2
  AND slot_id = $3
  AND lower(guest_email) = lower(sqlc.arg(guest_email))
  AND status = 'active'
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz
LIMIT 1;

-- name: CreateWaitlistEntry :one
INSERT INTO waitlist_entries (
  location_id,
  service_id,
  slot_id,
  guest_name,
  guest_email,
  guest_phone,
  practitioner_id,
  preferred_date,
  status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, 'active'
)
RETURNING *;

-- name: CountActiveWaitlistEntriesForSlot :one
SELECT COUNT(*)::int4 AS count
FROM waitlist_entries
WHERE slot_id = $1
  AND status = 'active'
  AND deleted_at = '0001-01-01 00:00:00Z'::timestamptz;

-- name: GetHostBookingAnalyticsSummary :one
WITH filtered AS (
  SELECT
    b.status,
    b.booked_at,
    s.start_at
  FROM bookings b
  JOIN appointment_slots s ON s.id = b.slot_id
  JOIN locations l ON l.id = b.location_id
  WHERE b.location_id = $1
    AND b.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
    AND s.deleted_at = '0001-01-01 00:00:00Z'::timestamptz
    AND (
      COALESCE(sqlc.narg(from_date)::text, '') = ''
      OR (s.start_at AT TIME ZONE l.timezone)::date >= sqlc.narg(from_date)::date
    )
    AND (
      COALESCE(sqlc.narg(to_date)::text, '') = ''
      OR (s.start_at AT TIME ZONE l.timezone)::date <= sqlc.narg(to_date)::date
    )
)
SELECT
  COUNT(*)::int4 AS total_count,
  COUNT(*) FILTER (WHERE status IN ('confirmed', 'completed'))::int4 AS filled_count,
  COUNT(*) FILTER (WHERE status = 'cancelled')::int4 AS cancelled_count,
  COUNT(*) FILTER (WHERE status = 'pending')::int4 AS pending_count,
  COUNT(*) FILTER (WHERE status = 'no_show')::int4 AS no_show_count,
  COALESCE(
    AVG(
      CASE
        WHEN status = 'pending' THEN EXTRACT(EPOCH FROM (now() - booked_at)) / 60.0
        ELSE NULL
      END
    )::float8,
    0
  ) AS pending_approval_avg_minutes
FROM filtered;
