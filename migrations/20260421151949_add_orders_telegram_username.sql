-- +goose Up

alter table orders
    add column if not exists telegram_username text;

create index if not exists ix_orders_telegram_username
    on orders (telegram_username);

-- +goose Down

drop index if exists ix_orders_telegram_username;

alter table orders
    drop column if exists telegram_username;