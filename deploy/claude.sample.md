# CLAUDE.md

Sample memory file baked into the Modal image so `looptap analyze` has
something to chew on when the hosted endpoint is invoked with no payload.
Intentionally rough — the whole point of `analyze` is to yell at files that
look like this.

## Project

This is a web app.

## Rules

- Write good code.
- Tests should pass.
- Be careful with the database.
- Don't forget error handling.
- Use the right patterns.

## Stack

We use a bunch of stuff. Mostly JavaScript. Some Python for scripts. Database
is Postgres (sometimes SQLite in dev). Redis for caching when we need it.

## Workflow

Open a PR, get a review, merge. Deploys happen automatically, usually.
