package domain

import (
	"errors"
	"time"
)

type RecipeType string

const (
	RecipePotion RecipeType = "potion"
	RecipeElixir RecipeType = "elixir"
	RecipeScroll RecipeType = "scroll"
	RecipeOther  RecipeType = "other"
)

type OrderStatus string

const (
	OrderStatusNew            OrderStatus = "new"
	OrderStatusInProgress     OrderStatus = "in_progress"
	OrderStatusReady          OrderStatus = "ready"
	OrderStatusCompleted      OrderStatus = "completed"
	OrderStatusCancelledUser  OrderStatus = "cancelled_user"
	OrderStatusCancelledAdmin OrderStatus = "cancelled_admin"
	OrderStatusExpired        OrderStatus = "expired"
)

type NotificationType string

const (
	NotificationTypeCraftStarted      NotificationType = "craft_started"
	NotificationTypeCraftReady        NotificationType = "craft_ready"
	NotificationTypeAdminOrderReady   NotificationType = "admin_order_ready"
	NotificationTypeAdminOrderClaimed NotificationType = "admin_order_claimed"
)

var (
	ErrCraftClosed        = errors.New("craft closed")
	ErrActiveOrderExists  = errors.New("active order exists")
	ErrRecipeNotFound     = errors.New("recipe not found")
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderInvalidStatus = errors.New("order invalid status")
	ErrForbidden          = errors.New("forbidden")
	ErrNoUserState        = errors.New("no user state")
	ErrBadInput           = errors.New("bad input")
)

type Recipe struct {
	Key        string
	Name       string
	MenuLabel  string
	Type       RecipeType
	UnitName   string
	Duration   time.Duration
	DefaultQty int
	AdminOnly  bool
}

type User struct {
	ID           int64
	TelegramID   int64
	Username     string
	FirstName    string
	LastName     string
	IsAdmin      bool
	LastNickname string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserState struct {
	UserID           int64
	Step             string
	PendingRecipeKey *string
	PendingNickname  *string
	UpdatedAt        time.Time
}

type CraftSettings struct {
	IsOpen    bool
	UpdatedAt time.Time
	UpdatedBy *int64
}

type Order struct {
	ID               int64
	UserID           *int64
	TelegramID       *int64
	Nickname         string
	RecipeKey        string
	RecipeName       string
	RecipeKind       RecipeType
	Qty              int
	Status           OrderStatus
	QueuePos         *int64
	CreatedAt        time.Time
	QueuedAt         time.Time
	StartedAt        *time.Time
	ReadyAt          *time.Time
	PickupDeadlineAt *time.Time
	CompletedAt      *time.Time
	CancelledAt      *time.Time
	AdminComment     *string
	Version          int
}

type Notification struct {
	ID          int64
	OrderID     int64
	TelegramID  int64
	Type        NotificationType
	ScheduledAt time.Time
	SentAt      *time.Time
	FailedAt    *time.Time
	ErrorText   *string
	Attempts    int
	CreatedAt   time.Time
}
