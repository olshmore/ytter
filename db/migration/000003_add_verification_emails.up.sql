CREATE TABLE "verification_emails" (
  "id" bigserial PRIMARY KEY,
  "email" varchar NOT NULL,
  "verification_token" varchar NOT NULL,
  "is_used" bool NOT NULL DEFAULT false,
  "created_at" timestamptz NOT NULL DEFAULT (now()),
  "token_expiry" timestamptz NOT NULL DEFAULT (now() + interval '15 minutes')
);

ALTER TABLE "users" ADD COLUMN "is_email_verified" bool NOT NULL DEFAULT false;