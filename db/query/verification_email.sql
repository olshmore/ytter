-- name: CreateVerificationEmail :one
INSERT INTO verification_emails (
    email,
    verification_token
) VALUES (
    $1, $2
) RETURNING *;

-- name: UpdateVerificationEmail :one
UPDATE verification_emails
SET
    is_used = TRUE
WHERE
    email = @email
    AND verification_token = @verification_token
    AND is_used = FALSE
    AND token_expiry > now()
RETURNING *;

-- name: GetVerificationEmailByToken :one
SELECT * FROM verification_emails
WHERE verification_token = $1 LIMIT 1;