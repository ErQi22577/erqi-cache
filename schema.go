package cache

import (
	"container/list"
	"time"
)

// Config Note that the default values of MaxBytes and ShardCount are 16G and 32,
// the default values of DefaultExpiration, CleanupInterval, and SavingInterval which means that
// the data never expire, The CleanupJanitor and SavingJanitor are not enabled by default.
type Config struct {
	ShardCount        int
	MaxBytes          int
	DefaultExpiration time.Duration
	CleanupInterval   time.Duration
	SavingInterval    time.Duration
}

type Item struct {
	V          any
	Ele        *list.Element
	IsDeleted  bool
	Expiration int64
	CreateTime int64
	UpdateTime int64
}
