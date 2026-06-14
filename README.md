# Padel Tournament Backend

Go API for a 12-player mixed-pair padel tournament using PostgreSQL/Neon.

## Setup

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

## Endpoints

- `GET /health`
- `GET /players`
- `POST /players`
- `POST /tournament/randomize`
- `GET /groups`
- `GET /matches`
- `POST /matches/{id}/result`

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
