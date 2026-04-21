-- +goose Up

drop index if exists ux_orders_one_active_per_user;

alter table orders
    alter column user_id drop not null,
    alter column telegram_id drop not null;

create unique index if not exists ux_orders_one_active_per_user
    on orders (telegram_id)
    where telegram_id is not null
        and status in ('new', 'in_progress', 'ready');

-- +goose Down

drop index if exists ux_orders_one_active_per_user;

alter table orders
    alter column user_id set not null,
    alter column telegram_id set not null;

create unique index if not exists ux_orders_one_active_per_user
    on orders (telegram_id)
    where status in ('new', 'in_progress', 'ready');