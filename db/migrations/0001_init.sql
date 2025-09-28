-- Schema for ledger Postgres storage
-- idempotent-ish: create if not exists where possible

create table if not exists users (
    id uuid primary key,
    email text
);

-- Accounts
create table if not exists accounts (
    id uuid primary key,
    user_id uuid not null,
    name text not null,
    currency char(3) not null,
    type text not null check (type in ('asset','liability','equity','revenue','expense')),
    "group" text not null,
    vendor text not null,
    metadata jsonb not null default '{}'::jsonb,
    system boolean not null default false,
    active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint fk_accounts_users foreign key (user_id) references users(id) on delete cascade
);

-- Uniqueness over (user, normalized path, currency)
create unique index if not exists ux_accounts_user_path_currency
on accounts (user_id, lower("group"), lower(vendor), type, upper(currency));

-- Entries
create table if not exists entries (
    id uuid primary key,
    user_id uuid not null,
    date timestamptz not null,
    currency char(3) not null,
    memo text not null default '',
    category text not null default 'uncategorized',
    metadata jsonb not null default '{}'::jsonb,
    is_reversed boolean not null default false,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint fk_entries_users foreign key (user_id) references users(id) on delete cascade
);

create index if not exists ix_entries_user_date_id on entries (user_id, date asc, id asc);

-- Lines
create table if not exists entry_lines (
    id uuid primary key,
    entry_id uuid not null,
    account_id uuid not null,
    side text not null check (side in ('debit','credit')),
    amount_minor bigint not null check (amount_minor > 0),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint fk_lines_entries foreign key (entry_id) references entries(id) on delete cascade,
    constraint fk_lines_accounts foreign key (account_id) references accounts(id) on delete restrict
);

create index if not exists ix_lines_entry on entry_lines (entry_id);
create index if not exists ix_lines_account on entry_lines (account_id);

-- Idempotency for entries
create table if not exists entry_idempotency (
    user_id uuid not null,
    key text not null,
    entry_id uuid not null,
    created_at timestamptz not null default now(),
    primary key (user_id, key),
    constraint fk_idem_entry foreign key (entry_id) references entries(id) on delete cascade
);
