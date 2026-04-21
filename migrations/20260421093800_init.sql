-- +goose Up
create type order_status as enum (
    'new',
    'in_progress',
    'ready',
    'completed',
    'cancelled_user',
    'cancelled_admin',
    'expired'
    );

create type recipe_kind as enum (
    'potion',
    'elixir',
    'scroll',
    'other'
    );

create type notification_type as enum (
    'craft_started',
    'craft_ready'
    );

create table if not exists users (
                                     id bigserial primary key,
                                     telegram_id bigint not null unique,
                                     username text,
                                     first_name text,
                                     last_name text,
                                     is_admin boolean not null default false,
                                     last_nickname text,
                                     created_at timestamptz not null default now(),
                                     updated_at timestamptz not null default now()
);

create table if not exists user_states (
                                           user_id bigint primary key references users(id) on delete cascade,
                                           step text not null,
                                           pending_recipe_key text,
                                           updated_at timestamptz not null default now()
);

create table if not exists craft_settings (
                                              id boolean primary key default true,
                                              is_open boolean not null default true,
                                              updated_at timestamptz not null default now(),
                                              updated_by bigint
);

insert into craft_settings (id, is_open)
values (true, true)
on conflict (id) do nothing;

create table if not exists orders (
                                      id bigserial primary key,
                                      user_id bigint not null references users(id),
                                      telegram_id bigint not null,
                                      nickname text not null,

                                      recipe_key text not null,
                                      recipe_name text not null,
                                      recipe_kind recipe_kind not null,
                                      qty integer not null,

                                      status order_status not null default 'new',

                                      queue_pos bigint,
                                      created_at timestamptz not null default now(),
                                      queued_at timestamptz not null default now(),

                                      started_at timestamptz,
                                      ready_at timestamptz,
                                      pickup_deadline_at timestamptz,
                                      completed_at timestamptz,
                                      cancelled_at timestamptz,

                                      admin_comment text,
                                      version integer not null default 1
);

create unique index if not exists ux_orders_one_active_per_user
    on orders (telegram_id)
    where status in ('new', 'in_progress', 'ready');

create unique index if not exists ux_orders_queue_pos_new
    on orders (queue_pos)
    where status = 'new';

create index if not exists ix_orders_status_queue
    on orders (status, queue_pos, created_at, id);

create index if not exists ix_orders_telegram_id
    on orders (telegram_id);

create table if not exists notifications (
                                             id bigserial primary key,
                                             order_id bigint not null references orders(id) on delete cascade,
                                             telegram_id bigint not null,
                                             type notification_type not null,
                                             scheduled_at timestamptz not null,
                                             sent_at timestamptz,
                                             failed_at timestamptz,
                                             error_text text,
                                             attempts integer not null default 0,
                                             created_at timestamptz not null default now(),
                                             unique(order_id, type)
);

create index if not exists ix_notifications_due
    on notifications (sent_at, scheduled_at);

-- +goose Down
drop table if exists notifications;
drop table if exists orders;
drop table if exists user_states;
drop table if exists craft_settings;
drop table if exists users;
drop type if exists notification_type;
drop type if exists recipe_kind;
drop type if exists order_status;