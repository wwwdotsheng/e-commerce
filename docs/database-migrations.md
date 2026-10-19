# Database schema migrations

The API does not create or change tables when it starts. PostgreSQL schema changes are reviewed, versioned SQL files in migrations/ and are applied with Atlas. This keeps schema updates explicit and prevents multiple API replicas from racing to alter the same database.

## Prerequisites

Install the [Atlas CLI](https://atlasgo.io/getting-started) and Docker. The Atlas configuration in scripts/atlas.hcl reads the target connection from DATABASE_URL; it uses an ephemeral PostgreSQL 15 container as the development database when planning changes.

Set DATABASE_URL in your shell or CI secret store. It must point to the database being migrated and should use a migration role with the required DDL privileges. For example, with local Compose PostgreSQL:

    export DATABASE_URL='postgres://postgres:<password>@localhost:5432/ecommerce?sslmode=disable'

Do not put real credentials in scripts/atlas.hcl, migration files, or committed .env files.

## Create a schema change

1. Update the GORM model in internal/model/ to describe the desired schema.
2. Generate a versioned migration from the repository root:

       cd scripts
       atlas migrate diff add_order_expiration --env gorm
       cd ..

3. Review the generated SQL in migrations/. Check how it handles existing rows, defaults, indexes, locks, and rollback or compatibility needs. Edit the SQL when the safe production sequence needs more control.
4. Verify migration checksums and run the full test suite:

       cd scripts
       atlas migrate hash --dir file://../migrations
       cd ..
       go test ./...

   The integration suite creates a disposable PostgreSQL database and applies every SQL migration in order before running API tests.
5. Commit the model and migration together. CI and reviewers can then see the exact schema transition.

Never edit a migration that has already been applied to a shared database. Add a new migration that moves the schema forward.

## Apply migrations

For a local Compose database, start PostgreSQL and apply pending migrations before starting the API:

    docker compose up -d --wait postgres
    export DATABASE_URL='postgres://postgres:<password>@localhost:5432/ecommerce?sslmode=disable'
    (cd scripts && atlas migrate apply --env gorm)
    docker compose up -d --build --wait

For Kubernetes or another deployment platform, run (cd scripts && atlas migrate apply --env gorm) as a single deployment job or controlled release step. Provide DATABASE_URL from your secret manager or Kubernetes Secret; do not mount it into every API replica just to perform migrations. Wait for migration success before rolling out the application version that depends on the new schema.

Atlas records applied versions in atlas_schema_revisions. Existing databases that already have this schema but are not tracked by Atlas need a one-time baseline: inspect and back up the database, confirm the exact migration history already represented by its schema, then follow Atlas's [baseline migration instructions](https://atlasgo.io/atlas-schema-migrations/baseline). Do not blindly apply the initial schema to an existing database.

## Compatibility and rollback

Prefer expand-and-contract changes for rolling deployments: add nullable columns or new tables first, deploy code that can work with both old and new schema, backfill data in a controlled job, then remove obsolete schema in a later release after old application versions are gone. Destructive changes and large backfills need an explicit backup, execution, monitoring, and recovery plan. Rolling back application code does not automatically roll back database migrations.
