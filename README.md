# Padel Tournament App

Go API and Astro website for a 10-player mixed-pair padel tournament using PostgreSQL.

## Quick Local Docker Setup

This is the recommended setup for a home server. It runs the website, Go API, and PostgreSQL together.

1. Create a Docker env file:

```bash
cp .env.docker.example .env.docker
```

Edit `.env.docker` and change `POSTGRES_PASSWORD`.

2. Start everything:

```bash
docker compose --env-file .env.docker up -d --build
```

The website and admin are available at:

```text
http://localhost:4321/
http://localhost:4321/admin/
```

The backend is proxied through the same web origin, so the frontend calls `/tournament`, `/players`, `/matches`, etc. without exposing or configuring a separate backend URL.

Health check:

```text
http://localhost:4321/health
```

On first database startup, Docker automatically applies `schema.sql` through `/docker-entrypoint-initdb.d`.

The API container uses this internal database URL:

```text
postgresql://padel:YOUR_PASSWORD@postgres:5432/padel?sslmode=disable
```

Useful Docker commands:

```bash
docker compose --env-file .env.docker ps
docker compose --env-file .env.docker logs api
docker compose --env-file .env.docker logs postgres
docker compose --env-file .env.docker exec postgres psql -U padel -d padel
docker compose --env-file .env.docker down
```

## Optional Neon Setup

If you deploy the API without the Docker Postgres service, you can use Neon instead.

1. Create a Neon PostgreSQL database.
2. Apply the schema:

```bash
psql "$DATABASE_URL" -f schema.sql
```

3. Configure environment variables:

```bash
cp .env.example .env
```

Set `DATABASE_URL` in `.env` to your Neon connection string. Do not expose this value in frontend code.

4. Run the API locally:

```bash
go mod tidy
go run .
```

The API listens on `PORT` or `8080` by default.

## Endpoints

- `GET /health`
- `GET /players`
- `POST /players`
- `PUT /players/{id}`
- `POST /tournament/reset`
- `POST /tournament/randomize`
- `GET /tournament`
- `GET /groups`
- `GET /results`
- `GET /bracket`
- `GET /champions`
- `GET /matches`
- `POST /matches/{id}/result`

Use `GET /tournament` for the full page payload. It returns:

```json
{
  "groups": [],
  "results": [],
  "bracket": {
    "semifinals": [],
    "final": null
  },
  "champions": {
    "champion": null,
    "runner_up": null,
    "final": null
  }
}
```

## Player Example

```bash
curl -X POST http://localhost:8080/players \
  -H 'Content-Type: application/json' \
  -d '{"first_name":"Pablo","last_name":"Alcolea","gender":"male","is_available":true}'
```

## Submit Match Result

```bash
curl -X POST http://localhost:8080/matches/MATCH_ID/result \
  -H 'Content-Type: application/json' \
  -d '{
    "sets": [
      { "pair1_games": 6, "pair2_games": 4 },
      { "pair1_games": 3, "pair2_games": 6 },
      { "pair1_games": 10, "pair2_games": 8 }
    ]
  }'
```

## Tournament Rules

- Exactly 5 available male and 5 available female players are required.
- Players are shuffled into 5 mixed pairs.
- All pairs are assigned to one group.
- The group plays a 5-pair round robin, 10 matches total.
- Top 4 pairs qualify.
- Semifinal 1: 1st vs 4th.
- Semifinal 2: 2nd vs 3rd.
- Third-place match: semifinal losers.
- Final: semifinal winners.

Group rankings are sorted by points, set difference, game difference, sets won, then games won.
