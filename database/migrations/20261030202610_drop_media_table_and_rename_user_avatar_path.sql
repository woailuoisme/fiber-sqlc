-- Modify "user" table
ALTER TABLE "user" RENAME COLUMN "image" TO "avatar_path";
-- Drop "media" table
DROP TABLE "media";
