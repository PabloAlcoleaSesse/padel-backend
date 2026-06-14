create extension if not exists pgcrypto;

do $$ begin
    create type player_gender as enum ('male', 'female');
exception
    when duplicate_object then null;
end $$;

do $$ begin
    create type match_round as enum ('group', 'semifinal', 'final');
exception
    when duplicate_object then null;
end $$;

do $$ begin
    create type match_status as enum ('scheduled', 'completed');
exception
    when duplicate_object then null;
end $$;

create table if not exists players (
    id uuid primary key default gen_random_uuid(),
    first_name text not null,
    last_name text not null,
    gender player_gender not null,
    is_available boolean not null default true,
    created_at timestamptz not null default now()
);

create table if not exists pairs (
    id uuid primary key default gen_random_uuid(),
    player1_id uuid not null references players(id) on delete restrict,
    player2_id uuid not null references players(id) on delete restrict,
    name text not null,
    created_at timestamptz not null default now(),
    constraint pairs_distinct_players check (player1_id <> player2_id)
);

create table if not exists groups (
    id uuid primary key default gen_random_uuid(),
    name text not null unique
);

create table if not exists group_pairs (
    id uuid primary key default gen_random_uuid(),
    group_id uuid not null references groups(id) on delete cascade,
    pair_id uuid not null references pairs(id) on delete cascade,
    unique (group_id, pair_id)
);

create table if not exists matches (
    id uuid primary key default gen_random_uuid(),
    group_id uuid references groups(id) on delete set null,
    pair1_id uuid not null references pairs(id) on delete restrict,
    pair2_id uuid not null references pairs(id) on delete restrict,
    round match_round not null,
    status match_status not null default 'scheduled',
    scheduled_at timestamptz,
    winner_pair_id uuid references pairs(id) on delete restrict,
    created_at timestamptz not null default now(),
    constraint matches_distinct_pairs check (pair1_id <> pair2_id)
);

create table if not exists match_sets (
    id uuid primary key default gen_random_uuid(),
    match_id uuid not null references matches(id) on delete cascade,
    set_number integer not null check (set_number > 0),
    pair1_games integer not null check (pair1_games >= 0),
    pair2_games integer not null check (pair2_games >= 0),
    unique (match_id, set_number)
);

create table if not exists group_standings (
    id uuid primary key default gen_random_uuid(),
    group_id uuid not null references groups(id) on delete cascade,
    pair_id uuid not null references pairs(id) on delete cascade,
    played integer not null default 0,
    wins integer not null default 0,
    losses integer not null default 0,
    sets_won integer not null default 0,
    sets_lost integer not null default 0,
    games_won integer not null default 0,
    games_lost integer not null default 0,
    points integer not null default 0,
    unique (group_id, pair_id)
);

create index if not exists idx_players_available_gender on players(is_available, gender);
create index if not exists idx_matches_round_status on matches(round, status);
create index if not exists idx_matches_group on matches(group_id);
create index if not exists idx_match_sets_match on match_sets(match_id);
