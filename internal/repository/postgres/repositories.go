package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"tg-craft-bot/internal/domain"

	"github.com/jackc/pgx/v5"
)

type UserRepository struct {
	db DBTX
}

func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) UpsertTelegramUser(
	ctx context.Context,
	telegramID int64,
	username, firstName, lastName string,
	defaultAdmin bool,
) (*domain.User, error) {
	q := `
insert into users (telegram_id, username, first_name, last_name, is_admin, updated_at)
values ($1, $2, $3, $4, $5, now())
on conflict (telegram_id) do update
set username = excluded.username,
    first_name = excluded.first_name,
    last_name = excluded.last_name,
    updated_at = now()
returning id, telegram_id, username, first_name, last_name, is_admin, coalesce(last_nickname, ''), created_at, updated_at
`
	var u domain.User
	err := r.db.QueryRow(ctx, q, telegramID, username, firstName, lastName, defaultAdmin).
		Scan(&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.IsAdmin, &u.LastNickname, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) SetLastNickname(ctx context.Context, userID int64, nickname string) error {
	_, err := r.db.Exec(ctx, `update users set last_nickname = $2, updated_at = now() where id = $1`, userID, nickname)
	if err != nil {
		return fmt.Errorf("set last nickname: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	q := `select id, telegram_id, username, first_name, last_name, is_admin, coalesce(last_nickname, ''), created_at, updated_at from users where telegram_id = $1`
	var u domain.User
	err := r.db.QueryRow(ctx, q, telegramID).
		Scan(&u.ID, &u.TelegramID, &u.Username, &u.FirstName, &u.LastName, &u.IsAdmin, &u.LastNickname, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrForbidden
		}
		return nil, fmt.Errorf("get user by telegram id: %w", err)
	}
	return &u, nil
}

type UserStateRepository struct {
	db DBTX
}

func NewUserStateRepository(db DBTX) *UserStateRepository {
	return &UserStateRepository{db: db}
}

func (r *UserStateRepository) SetAwaitingNickname(ctx context.Context, userID int64, recipeKey string) error {
	q := `
insert into user_states (user_id, step, pending_recipe_key, pending_nickname, updated_at)
values ($1, 'awaiting_nickname', $2, null, now())
on conflict (user_id) do update
set step = excluded.step,
    pending_recipe_key = excluded.pending_recipe_key,
    pending_nickname = excluded.pending_nickname,
    updated_at = now()
`
	_, err := r.db.Exec(ctx, q, userID, recipeKey)
	if err != nil {
		return fmt.Errorf("set awaiting nickname: %w", err)
	}
	return nil
}

func (r *UserStateRepository) SetConfirmOrder(ctx context.Context, userID int64, recipeKey, nickname string) error {
	q := `
insert into user_states (user_id, step, pending_recipe_key, pending_nickname, updated_at)
values ($1, 'confirm_order', $2, $3, now())
on conflict (user_id) do update
set step = excluded.step,
    pending_recipe_key = excluded.pending_recipe_key,
    pending_nickname = excluded.pending_nickname,
    updated_at = now()
`
	_, err := r.db.Exec(ctx, q, userID, recipeKey, nickname)
	if err != nil {
		return fmt.Errorf("set confirm order state: %w", err)
	}
	return nil
}

func (r *UserStateRepository) SetAdminSingleOrder(ctx context.Context, userID int64, recipeKey string) error {
	q := `
insert into user_states (user_id, step, pending_recipe_key, pending_nickname, updated_at)
values ($1, 'admin_single_order_nickname', $2, null, now())
on conflict (user_id) do update
set step = excluded.step,
    pending_recipe_key = excluded.pending_recipe_key,
    pending_nickname = excluded.pending_nickname,
    updated_at = now()
`
	_, err := r.db.Exec(ctx, q, userID, recipeKey)
	if err != nil {
		return fmt.Errorf("set admin single order state: %w", err)
	}
	return nil
}

func (r *UserStateRepository) SetAdminBulkOrder(ctx context.Context, userID int64, recipeKey string) error {
	q := `
insert into user_states (user_id, step, pending_recipe_key, pending_nickname, updated_at)
values ($1, 'admin_bulk_order_nicknames', $2, null, now())
on conflict (user_id) do update
set step = excluded.step,
    pending_recipe_key = excluded.pending_recipe_key,
    pending_nickname = excluded.pending_nickname,
    updated_at = now()
`
	_, err := r.db.Exec(ctx, q, userID, recipeKey)
	if err != nil {
		return fmt.Errorf("set admin bulk order state: %w", err)
	}
	return nil
}

func (r *UserStateRepository) Get(ctx context.Context, userID int64) (*domain.UserState, error) {
	q := `select user_id, step, pending_recipe_key, pending_nickname, updated_at from user_states where user_id = $1`

	var s domain.UserState
	err := r.db.QueryRow(ctx, q, userID).Scan(
		&s.UserID,
		&s.Step,
		&s.PendingRecipeKey,
		&s.PendingNickname,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNoUserState
		}
		return nil, fmt.Errorf("get user state: %w", err)
	}

	return &s, nil
}

func (r *UserStateRepository) Clear(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `delete from user_states where user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("clear user state: %w", err)
	}
	return nil
}

type SettingsRepository struct {
	db DBTX
}

func NewSettingsRepository(db DBTX) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(ctx context.Context) (*domain.CraftSettings, error) {
	q := `select is_open, updated_at, updated_by from craft_settings where id = true`
	var s domain.CraftSettings
	err := r.db.QueryRow(ctx, q).Scan(&s.IsOpen, &s.UpdatedAt, &s.UpdatedBy)
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	return &s, nil
}

func (r *SettingsRepository) SetOpen(ctx context.Context, isOpen bool, updatedBy int64) error {
	q := `update craft_settings set is_open = $1, updated_at = now(), updated_by = $2 where id = true`
	_, err := r.db.Exec(ctx, q, isOpen, updatedBy)
	if err != nil {
		return fmt.Errorf("set craft open: %w", err)
	}
	return nil
}

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) HasActiveOrder(ctx context.Context, telegramID int64) (bool, error) {
	q := `select exists(select 1 from orders where telegram_id = $1 and status in ('new', 'in_progress', 'ready'))`
	var exists bool
	if err := r.db.QueryRow(ctx, q, telegramID).Scan(&exists); err != nil {
		return false, fmt.Errorf("has active order: %w", err)
	}
	return exists, nil
}

func (r *OrderRepository) nextQueuePos(ctx context.Context) (int64, error) {
	q := `select coalesce(max(queue_pos), 0) + 1 from orders where status = 'new'`
	var pos int64
	if err := r.db.QueryRow(ctx, q).Scan(&pos); err != nil {
		return 0, fmt.Errorf("next queue pos: %w", err)
	}
	return pos, nil
}

func (r *OrderRepository) CreateAtTail(ctx context.Context, o *domain.Order) (*domain.Order, error) {
	pos, err := r.nextQueuePos(ctx)
	if err != nil {
		return nil, err
	}

	q := `
insert into orders (
    user_id, telegram_id, nickname, recipe_key, recipe_name, recipe_kind, qty,
    status, queue_pos, created_at, queued_at, version
) values ($1,$2,$3,$4,$5,$6,$7,'new',$8,now(),now(),1)
returning id, user_id, telegram_id, nickname, recipe_key, recipe_name, recipe_kind, qty,
          status, queue_pos, created_at, queued_at, started_at, ready_at, pickup_deadline_at,
          completed_at, cancelled_at, admin_comment, version
`

	var out domain.Order
	err = r.db.QueryRow(
		ctx,
		q,
		o.UserID,
		o.TelegramID,
		o.Nickname,
		o.RecipeKey,
		o.RecipeName,
		o.RecipeKind,
		o.Qty,
		pos,
	).Scan(
		&out.ID,
		&out.UserID,
		&out.TelegramID,
		&out.Nickname,
		&out.RecipeKey,
		&out.RecipeName,
		&out.RecipeKind,
		&out.Qty,
		&out.Status,
		&out.QueuePos,
		&out.CreatedAt,
		&out.QueuedAt,
		&out.StartedAt,
		&out.ReadyAt,
		&out.PickupDeadlineAt,
		&out.CompletedAt,
		&out.CancelledAt,
		&out.AdminComment,
		&out.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	// Для только что созданного заказа можно вернуть username из входной модели,
	// если сервис его уже знает. В БД оно не хранится в orders, а дальше будет
	// подставляться через JOIN users.username.
	out.TelegramUsername = o.TelegramUsername

	return &out, nil
}

func (r *OrderRepository) GetActiveByTelegramID(ctx context.Context, telegramID int64) (*domain.Order, error) {
	q := `
select o.id, o.user_id, o.telegram_id, u.username, o.nickname, o.recipe_key, o.recipe_name, o.recipe_kind, o.qty, o.status,
       o.queue_pos, o.created_at, o.queued_at, o.started_at, o.ready_at, o.pickup_deadline_at,
       o.completed_at, o.cancelled_at, o.admin_comment, o.version
from orders o
left join users u on u.id = o.user_id
where o.telegram_id = $1 and o.status in ('new', 'in_progress', 'ready')
order by o.created_at desc
limit 1
`
	return scanOrder(r.db.QueryRow(ctx, q, telegramID))
}

func (r *OrderRepository) GetByID(ctx context.Context, id int64) (*domain.Order, error) {
	q := `
select o.id, o.user_id, o.telegram_id, u.username, o.nickname, o.recipe_key, o.recipe_name, o.recipe_kind, o.qty, o.status,
       o.queue_pos, o.created_at, o.queued_at, o.started_at, o.ready_at, o.pickup_deadline_at,
       o.completed_at, o.cancelled_at, o.admin_comment, o.version
from orders o
left join users u on u.id = o.user_id
where o.id = $1
`
	return scanOrder(r.db.QueryRow(ctx, q, id))
}

func (r *OrderRepository) GetByIDForUpdate(ctx context.Context, id int64) (*domain.Order, error) {
	q := `
select o.id,
       o.user_id,
       o.telegram_id,
       (select u.username from users u where u.id = o.user_id) as telegram_username,
       o.nickname,
       o.recipe_key,
       o.recipe_name,
       o.recipe_kind,
       o.qty,
       o.status,
       o.queue_pos,
       o.created_at,
       o.queued_at,
       o.started_at,
       o.ready_at,
       o.pickup_deadline_at,
       o.completed_at,
       o.cancelled_at,
       o.admin_comment,
       o.version
from orders o
where o.id = $1
for update of o
`
	return scanOrder(r.db.QueryRow(ctx, q, id))
}

func (r *OrderRepository) CancelByUser(ctx context.Context, id int64) error {
	q := `
update orders
set status = 'cancelled_user',
    queue_pos = null,
    cancelled_at = now(),
    version = version + 1
where id = $1 and status in ('new', 'in_progress', 'ready')
`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("cancel by user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOrderInvalidStatus
	}
	return nil
}

func (r *OrderRepository) CancelByAdmin(ctx context.Context, id int64) error {
	q := `
update orders
set status = 'cancelled_admin',
    queue_pos = null,
    cancelled_at = now(),
    version = version + 1
where id = $1 and status in ('new', 'in_progress', 'ready')
`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("cancel by admin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOrderInvalidStatus
	}
	return nil
}

func (r *OrderRepository) MarkInProgress(ctx context.Context, id int64, startedAt, readyAt, pickupDeadline time.Time) error {
	q := `
update orders
set status = 'in_progress',
    queue_pos = null,
    started_at = $2,
    ready_at = $3,
    pickup_deadline_at = $4,
    version = version + 1
where id = $1 and status = 'new'
`
	tag, err := r.db.Exec(ctx, q, id, startedAt, readyAt, pickupDeadline)
	if err != nil {
		return fmt.Errorf("mark in progress: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOrderInvalidStatus
	}
	return nil
}

func (r *OrderRepository) MarkReady(ctx context.Context, id int64) error {
	q := `
update orders
set status = 'ready',
    version = version + 1
where id = $1 and status = 'in_progress'
`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark ready: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOrderInvalidStatus
	}
	return nil
}

func (r *OrderRepository) MarkCompleted(ctx context.Context, id int64) error {
	q := `
update orders
set status = 'completed',
    completed_at = now(),
    version = version + 1
where id = $1 and status in ('ready', 'in_progress')
`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark completed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrOrderInvalidStatus
	}
	return nil
}

func (r *OrderRepository) ListQueue(ctx context.Context) ([]domain.Order, error) {
	q := `
select o.id, o.user_id, o.telegram_id, u.username, o.nickname, o.recipe_key, o.recipe_name, o.recipe_kind, o.qty, o.status,
       o.queue_pos, o.created_at, o.queued_at, o.started_at, o.ready_at, o.pickup_deadline_at,
       o.completed_at, o.cancelled_at, o.admin_comment, o.version
from orders o
left join users u on u.id = o.user_id
where o.status = 'new'
order by o.queue_pos asc, o.created_at asc, o.id asc
`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list queue: %w", err)
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (r *OrderRepository) ListQueueByKinds(ctx context.Context, kinds ...domain.RecipeType) ([]domain.Order, error) {
	if len(kinds) == 0 {
		return r.ListQueue(ctx)
	}

	args := make([]string, 0, len(kinds))
	for _, k := range kinds {
		args = append(args, string(k))
	}

	q := `
select o.id, o.user_id, o.telegram_id, u.username, o.nickname, o.recipe_key, o.recipe_name, o.recipe_kind, o.qty, o.status,
       o.queue_pos, o.created_at, o.queued_at, o.started_at, o.ready_at, o.pickup_deadline_at,
       o.completed_at, o.cancelled_at, o.admin_comment, o.version
from orders o
left join users u on u.id = o.user_id
where o.status = 'new'
  and o.recipe_kind = any($1)
order by o.queue_pos asc, o.created_at asc, o.id asc
`
	rows, err := r.db.Query(ctx, q, args)
	if err != nil {
		return nil, fmt.Errorf("list queue by kinds: %w", err)
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (r *OrderRepository) ListByStatus(ctx context.Context, statuses ...domain.OrderStatus) ([]domain.Order, error) {
	if len(statuses) == 0 {
		return nil, nil
	}

	q := `
select o.id, o.user_id, o.telegram_id, u.username, o.nickname, o.recipe_key, o.recipe_name, o.recipe_kind, o.qty, o.status,
       o.queue_pos, o.created_at, o.queued_at, o.started_at, o.ready_at, o.pickup_deadline_at,
       o.completed_at, o.cancelled_at, o.admin_comment, o.version
from orders o
left join users u on u.id = o.user_id
where o.status = any($1)
order by o.created_at asc, o.id asc
`
	args := make([]string, 0, len(statuses))
	for _, s := range statuses {
		args = append(args, string(s))
	}

	rows, err := r.db.Query(ctx, q, args)
	if err != nil {
		return nil, fmt.Errorf("list by status: %w", err)
	}
	defer rows.Close()

	var out []domain.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	return out, rows.Err()
}

func (r *OrderRepository) MoveUp(ctx context.Context, id int64) error {
	order, err := r.GetByIDForUpdate(ctx, id)
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusNew || order.QueuePos == nil {
		return domain.ErrOrderInvalidStatus
	}

	q := `
select id, queue_pos
from orders
where status = 'new' and queue_pos < $1
order by queue_pos desc
limit 1
for update
`
	var otherID int64
	var otherPos int64
	err = r.db.QueryRow(ctx, q, *order.QueuePos).Scan(&otherID, &otherPos)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("find previous order: %w", err)
	}

	if err := r.swapQueue(ctx, id, *order.QueuePos, otherID, otherPos); err != nil {
		return err
	}
	return nil
}

func (r *OrderRepository) MoveDown(ctx context.Context, id int64) error {
	order, err := r.GetByIDForUpdate(ctx, id)
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusNew || order.QueuePos == nil {
		return domain.ErrOrderInvalidStatus
	}

	q := `
select id, queue_pos
from orders
where status = 'new' and queue_pos > $1
order by queue_pos asc
limit 1
for update
`
	var otherID int64
	var otherPos int64
	err = r.db.QueryRow(ctx, q, *order.QueuePos).Scan(&otherID, &otherPos)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("find next order: %w", err)
	}

	if err := r.swapQueue(ctx, id, *order.QueuePos, otherID, otherPos); err != nil {
		return err
	}
	return nil
}

func (r *OrderRepository) MoveToTail(ctx context.Context, id int64) error {
	order, err := r.GetByIDForUpdate(ctx, id)
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusNew || order.QueuePos == nil {
		return domain.ErrOrderInvalidStatus
	}

	lastPos, err := r.nextQueuePos(ctx)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `update orders set queue_pos = $2, version = version + 1 where id = $1`, id, lastPos)
	if err != nil {
		return fmt.Errorf("move to tail: %w", err)
	}
	return nil
}

func (r *OrderRepository) MoveToHead(ctx context.Context, id int64) error {
	order, err := r.GetByIDForUpdate(ctx, id)
	if err != nil {
		return err
	}
	if order.Status != domain.OrderStatusNew || order.QueuePos == nil {
		return domain.ErrOrderInvalidStatus
	}

	_, err = r.db.Exec(ctx, `update orders set queue_pos = queue_pos + 1 where status = 'new' and id <> $1`, id)
	if err != nil {
		return fmt.Errorf("shift queue for head: %w", err)
	}

	_, err = r.db.Exec(ctx, `update orders set queue_pos = 1, version = version + 1 where id = $1`, id)
	if err != nil {
		return fmt.Errorf("set head pos: %w", err)
	}
	return nil
}

func (r *OrderRepository) NormalizeQueue(ctx context.Context) error {
	q := `
with ranked as (
    select id, row_number() over(order by queue_pos asc, created_at asc, id asc) as rn
    from orders
    where status = 'new'
)
update orders o
set queue_pos = ranked.rn
from ranked
where o.id = ranked.id
`
	_, err := r.db.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("normalize queue: %w", err)
	}
	return nil
}

func (r *OrderRepository) swapQueue(ctx context.Context, id1, pos1, id2, pos2 int64) error {
	_, err := r.db.Exec(ctx, `update orders set queue_pos = -1 where id = $1`, id1)
	if err != nil {
		return fmt.Errorf("temp swap queue: %w", err)
	}
	_, err = r.db.Exec(ctx, `update orders set queue_pos = $2 where id = $1`, id2, pos1)
	if err != nil {
		return fmt.Errorf("swap queue second: %w", err)
	}
	_, err = r.db.Exec(ctx, `update orders set queue_pos = $2, version = version + 1 where id = $1`, id1, pos2)
	if err != nil {
		return fmt.Errorf("swap queue first: %w", err)
	}
	return nil
}

type NotificationRepository struct {
	db DBTX
}

func NewNotificationRepository(db DBTX) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(ctx context.Context, n domain.Notification) error {
	q := `
insert into notifications (order_id, telegram_id, type, scheduled_at)
values ($1, $2, $3, $4)
on conflict (order_id, type) do nothing
`
	_, err := r.db.Exec(ctx, q, n.OrderID, n.TelegramID, n.Type, n.ScheduledAt)
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

func (r *NotificationRepository) LockDue(ctx context.Context, limit int) ([]domain.Notification, error) {
	q := `
select id, order_id, telegram_id, type, scheduled_at, sent_at, failed_at, error_text, attempts, created_at
from notifications
where sent_at is null
  and scheduled_at <= now()
order by scheduled_at asc, id asc
for update skip locked
limit $1
`
	rows, err := r.db.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("lock due notifications: %w", err)
	}
	defer rows.Close()

	var out []domain.Notification
	for rows.Next() {
		var n domain.Notification
		if err := rows.Scan(
			&n.ID, &n.OrderID, &n.TelegramID, &n.Type, &n.ScheduledAt,
			&n.SentAt, &n.FailedAt, &n.ErrorText, &n.Attempts, &n.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *NotificationRepository) MarkSent(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `update notifications set sent_at = now(), error_text = null where id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark sent: %w", err)
	}
	return nil
}

func (r *NotificationRepository) MarkFailed(ctx context.Context, id int64, errText string) error {
	_, err := r.db.Exec(ctx, `update notifications set failed_at = now(), attempts = attempts + 1, error_text = $2 where id = $1`, id, errText)
	if err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}
	return nil
}

func scanOrder(row interface {
	Scan(dest ...any) error
}) (*domain.Order, error) {
	var o domain.Order
	err := row.Scan(
		&o.ID,
		&o.UserID,
		&o.TelegramID,
		&o.TelegramUsername,
		&o.Nickname,
		&o.RecipeKey,
		&o.RecipeName,
		&o.RecipeKind,
		&o.Qty,
		&o.Status,
		&o.QueuePos,
		&o.CreatedAt,
		&o.QueuedAt,
		&o.StartedAt,
		&o.ReadyAt,
		&o.PickupDeadlineAt,
		&o.CompletedAt,
		&o.CancelledAt,
		&o.AdminComment,
		&o.Version,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrOrderNotFound
		}
		return nil, fmt.Errorf("scan order: %w", err)
	}
	return &o, nil
}
