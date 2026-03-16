-- name: CreateUser :one
INSERT INTO users (
  username,
  hashed_password,
  first_name,
  last_name,
  email,
  role
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY id
LIMIT $1
OFFSET $2;

-- name: GetUser :one
SELECT * FROM users
WHERE id = $1 LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1 LIMIT 1;

-- name: GetUserByGoogleID :one
SELECT * FROM users
WHERE google_id = $1 LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1 LIMIT 1;

-- name: UpdateUser :one
UPDATE users
SET
  hashed_password = COALESCE(sqlc.narg(hashed_password), hashed_password),
  password_changed_at = COALESCE(sqlc.narg(password_changed_at), password_changed_at),
  first_name = COALESCE(sqlc.narg(first_name), first_name),
  last_name = COALESCE(sqlc.narg(last_name), last_name),
  email = COALESCE(sqlc.narg(email), email),
  is_email_verified = COALESCE(sqlc.narg(is_email_verified), is_email_verified),
  role = COALESCE(sqlc.narg(role), role),
  google_id = COALESCE(sqlc.narg(google_id), google_id)
WHERE
  username = sqlc.arg(username)
RETURNING *;

-- name: UpdateUserEmailVerified :one
UPDATE users
SET
  is_email_verified = COALESCE(sqlc.narg(is_email_verified), is_email_verified)
WHERE
  email = sqlc.arg(email)
RETURNING *;