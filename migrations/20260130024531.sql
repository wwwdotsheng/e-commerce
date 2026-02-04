-- Create "users" table
CREATE TABLE "public"."users" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "user_name" character varying(30) NOT NULL,
  "email" character varying(30) NOT NULL,
  "password" character varying(255) NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_users_deleted_at" to table: "users"
CREATE INDEX "idx_users_deleted_at" ON "public"."users" ("deleted_at");
-- Create index "uni_user_user_email" to table: "users"
CREATE UNIQUE INDEX "uni_user_user_email" ON "public"."users" ("email");
-- Create index "uni_user_user_name" to table: "users"
CREATE UNIQUE INDEX "uni_user_user_name" ON "public"."users" ("user_name");
