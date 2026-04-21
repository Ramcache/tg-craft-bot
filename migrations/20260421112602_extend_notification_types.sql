-- +goose Up

alter type notification_type add value if not exists 'admin_order_ready';
alter type notification_type add value if not exists 'admin_order_claimed';

-- +goose Down
-- PostgreSQL enum values назад безопасно не удаляются без пересоздания типа.