# Padel Tournament Backend

Go API for a 12-player mixed-pair padel tournament using PostgreSQL/Neon.

## Neon Setup

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

4. Run locally:

```bash
go mod tidy
go run .
```

The API listens on `PORT` or `8080` by default.

## Docker PostgreSQL Setup

For a home server, run PostgreSQL in Docker and point the API at it.

1. Create a Docker env file:

```bash
cp .env.docker.example .env.docker
```

Edit `.env.docker` and change `POSTGRES_PASSWORD`.

2. Start PostgreSQL:

```bash
docker compose --env-file .env.docker up -d
```

On first startup, Docker automatically applies `schema.sql` through `/docker-entrypoint-initdb.d`.

3. Run the API against Docker PostgreSQL:

```bash
export DATABASE_URL='postgresql://padel:YOUR_PASSWORD@localhost:5432/padel?sslmode=disable'
go run .
```

If the API is also running in Docker on the same Compose network, use host `postgres` instead of `localhost`:

```text
postgresql://padel:YOUR_PASSWORD@postgres:5432/padel?sslmode=disable
```

Useful database commands:

```bash
docker compose --env-file .env.docker ps
docker compose --env-file .env.docker logs postgres
docker compose --env-file .env.docker exec postgres psql -U padel -d padel
```

## Endpoints

- `GET /health`
- `GET /players`
- `POST /players`
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

- Exactly 6 available male and 6 available female players are required.
- Players are shuffled into 6 mixed pairs.
- Pairs are split into Group A and Group B.
- Each group plays a 3-pair round robin.
- Top 2 pairs per group qualify.
- Semifinal 1: Group A 1st vs Group B 2nd.
- Semifinal 2: Group B 1st vs Group A 2nd.
- Final: semifinal winners.

Group rankings are sorted by points, set difference, game difference, sets won, then games won.
