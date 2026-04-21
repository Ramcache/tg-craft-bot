-- +goose Up

alter table user_states
    add column if not exists pending_nickname text;

-- +goose Down

alter table user_states
    drop column if exists pending_nickname;