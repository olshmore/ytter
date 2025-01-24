DROP TABLE IF EXISTS "verification_emails" CASCADE;

ALTER TABLE "users" DROP COLUMN "is_email_verified";