package cache

import (
	"container/list"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	DefaultExpiration time.Duration = 0
	MaxBytes                        = 17179869184 // 16G
	ShardCount                      = 32
	prime32                         = 16777619
)

type Cache struct {
	*cache
}

type cache struct {
	database          database
	defaultExpiration time.Duration
	CleanupJanitor    *Janitor
	SavingJanitor     *Janitor
	ll                *list.List
	shardCount        int
	maxBytes          int
	nowBytes          int
	onEvicted         func(key string, value any)
	onExited          func()
}

type database []*SharedMap

type SharedMap struct {
	m map[string]*atomic.Value
	sync.RWMutex
}

type Item struct {
	V          any
	Ele        *list.Element
	IsDeleted  bool
	Expiration int64
	CreateTime int64
	UpdateTime int64
}

// New create Cache with Config
func New(config Config) *Cache {
	c := &cache{}
	C := &Cache{c}

	c.ll = &list.List{}
	c.onEvicted = func(string, any) {}
	c.defaultExpiration = config.DefaultExpiration

	var m database
	if config.ShardCount > 0 {
		c.shardCount = config.ShardCount
		m = make(database, config.ShardCount)
	} else {
		c.shardCount = ShardCount
		m = make(database, ShardCount)
	}
	for i := 0; i < c.shardCount; i++ {
		m[i] = &SharedMap{
			m: make(map[string]*atomic.Value),
		}
	}
	c.database = m

	if config.MaxBytes > 0 {
		c.maxBytes = config.MaxBytes
	} else {
		c.maxBytes = MaxBytes
	}

	if config.CleanupInterval > 0 {
		j := &Janitor{
			Interval: config.CleanupInterval,
			StopChan: make(chan bool),
			Type:     Cleanup,
		}
		c.CleanupJanitor = j
		go c.RunJanitor(Cleanup)
	}
	if config.SavingInterval > 0 {
		j := &Janitor{
			Interval: config.SavingInterval,
			StopChan: make(chan bool),
			Type:     Saving,
		}
		c.SavingJanitor = j
		go c.RunJanitor(Saving)
	}

	runtime.SetFinalizer(C, (*Cache).finalizer)
	return C
}

// NewFrom Encapsulates the New and c.ItemInit(). You can also use them separately.
// Note that the items parameter needs to be generated in advance using the c.Items().
// It is not recommended to pass custom items directly, as this may cause unknown errors.
func NewFrom(config Config, items map[string]Item) *Cache {
	C := New(config)
	C.ItemsInit(items)
	return C
}

// Items Exports all data in the cache for data persistence or data migration.
func (c *cache) Items() map[string]Item {
	m := make(map[string]Item, c.ll.Len())
	now := time.Now().UnixNano()
	for _, db := range c.database {
		for k, v := range db.m {
			item := v.Load().(*Item)
			if item.Expiration > 0 && now > item.Expiration {
				continue
			}
			m[k] = *item
		}
	}
	return m
}

// ItemsInit writes the data exported by the c.Items() into the cache.
func (c *cache) ItemsInit(items map[string]Item) {
	for k, v := range items {
		sharedMap := c.GetShardMap(k)
		sharedMap.Lock()

		sharedMap.m[k] = &atomic.Value{}
		ele := c.ll.PushFront(k)
		v.Ele = ele
		sharedMap.m[k].Store(&v)
		c.nowBytes += int(unsafe.Sizeof(sharedMap))

		sharedMap.Unlock()
	}
}

// Flush Clear all data in the cache.
func (c *cache) Flush() {
	c.ll = &list.List{}
	for i, n := 0, len(c.database); i < n; i++ {
		old := c.database[i]
		old.Lock()

		c.database[i] = &SharedMap{
			m: make(map[string]*atomic.Value),
		}

		old.Unlock()
	}
}

// Add k key, x value, d defaultExpiration(0:Never expire.).
func (c *cache) Add(k string, x any, d time.Duration, overWrite bool) error {
	sharedMap := c.GetShardMap(k)
	sharedMap.Lock()

	_, found := sharedMap.m[k]
	if found && !overWrite {
		sharedMap.Unlock()
		return fmt.Errorf("Item %s already exists\n", k)
	}

	sharedMap.m[k] = &atomic.Value{}
	mk := sharedMap.m[k]
	sharedMap.Unlock()

	ele := c.ll.PushFront(k)
	sizeBefore := int(unsafe.Sizeof(sharedMap))
	mk.Store(&Item{
		V:          x,
		Ele:        ele,
		IsDeleted:  false,
		Expiration: c.calExpiration(d),
		CreateTime: time.Now().UnixNano(),
		UpdateTime: time.Now().UnixNano(),
	})
	sizeAfter := int(unsafe.Sizeof(sharedMap))
	c.nowBytes += sizeAfter - sizeBefore

	return nil
}

// Delete really remove item.
func (c *cache) Delete(k string) {
	sharedMap := c.GetShardMap(k)
	sharedMap.Lock()

	v, found := sharedMap.m[k]
	if found {
		item, ok := v.Load().(*Item)
		if !ok {
			sharedMap.Unlock()
			return
		}

		c.ll.Remove(item.Ele)
		delete(sharedMap.m, k)
		sharedMap.Unlock()
		c.onEvicted(k, v.Load().(*Item).V)
	} else {
		sharedMap.Unlock()
		return
	}
}

// UnsafeDelete only modifies item.IsDeleted to make it invisible.
func (c *cache) UnsafeDelete(k string) {
	v, found := c.get4update(k)
	if !found {
		return
	}
	v.IsDeleted = true
	c.updateUpdateTime(v)
	c.onEvicted(k, v.V)
}

// Update alter item.V if item can be found.
func (c *cache) Update(k string, x any) error {
	v, found := c.get4update(k)
	if !found || v.IsDeleted {
		return fmt.Errorf("Item %s not found\n", k)
	}
	v.V = x
	c.updateUpdateTime(v)
	return nil
}

// UpdateExpiration alter item.ExpirationTime if item can be found.
func (c *cache) UpdateExpiration(k string, d time.Duration) error {
	v, found := c.get4update(k)
	if !found || v.IsDeleted {
		return fmt.Errorf("Item %s not found\n", k)
	}
	v.Expiration = c.calExpiration(d)
	c.updateUpdateTime(v)
	return nil
}

// Get return item.V, isFound.
// If item has expired or isDeleted is true, will not be found.
func (c *cache) Get(k string) (any, bool) {
	v, found := c.get4Get(k)
	if !found {
		return nil, false
	}
	return v.V, true
}

// UnsafeGet return item.V, isExist.
// It will also return data that has expired and isDeleted is true.
func (c *cache) UnsafeGet(k string) (any, bool) {
	v, found := c.get4Get(k)
	if !found {
		return nil, false
	} else {
		return v.V, true
	}
}

// GetWithExpiration return item.V, item.ExpirationTime, isFound.
func (c *cache) GetWithExpiration(k string) (any, time.Time, bool) {
	v, found := c.get4Get(k)
	if !found {
		return nil, time.Time{}, false
	}
	return v.V, time.Unix(0, v.Expiration), true
}

// GetCreateTime return item.CreateTime, isFound.
func (c *cache) GetCreateTime(k string) (time.Time, bool) {
	v, found := c.get4Get(k)
	if !found {
		return time.Time{}, false
	}
	return time.Unix(0, v.CreateTime), true
}

// GetUpdateTime return item.UpdateTime, isFound.
func (c *cache) GetUpdateTime(k string) (time.Time, bool) {
	v, found := c.get4Get(k)
	if !found {
		return time.Time{}, false
	}
	return time.Unix(0, v.UpdateTime), true
}

// Increment for int, item.V + n.
func (c *cache) Increment(k string, n int64) error {
	item, found := c.get4update(k)
	if !found {
		return fmt.Errorf("Item %s not found\n", k)
	}
	V := item.V
	switch v := V.(type) {
	case int:
		item.V = v + int(n)
	case int8:
		item.V = v + int8(n)
	case int16:
		item.V = v + int16(n)
	case int32:
		item.V = v + int32(n)
	case int64:
		item.V = v + n
	case uint:
		item.V = v + uint(n)
	case uintptr:
		item.V = v + uintptr(n)
	case uint8:
		item.V = v + uint8(n)
	case uint16:
		item.V = v + uint16(n)
	case uint32:
		item.V = v + uint32(n)
	case uint64:
		item.V = v + uint64(n)
	case float32:
		item.V = v + float32(n)
	case float64:
		item.V = v + float64(n)
	default:
		return fmt.Errorf("The value for %s is not an integer or float\n", k)
	}
	return nil
}

// IncrementFloat for float, item.V + n.
func (c *cache) IncrementFloat(k string, n float64) error {
	item, found := c.get4update(k)
	if !found {
		return fmt.Errorf("Item %s not found\n", k)
	}
	V := item.V
	switch v := V.(type) {
	case float64:
		item.V = v + n
	case float32:
		item.V = v + float32(n)
	default:
		return fmt.Errorf("The value for %s does not have type float32 or float64\n", k)
	}
	return nil
}

// Decrement for int, item.V - n.
func (c *cache) Decrement(k string, n int64) error {
	item, found := c.get4update(k)
	if !found {
		return fmt.Errorf("Item %s not found\n", k)
	}
	V := item.V
	switch v := V.(type) {
	case int:
		item.V = v - int(n)
	case int8:
		item.V = v - int8(n)
	case int16:
		item.V = v - int16(n)
	case int32:
		item.V = v - int32(n)
	case int64:
		item.V = v - n
	case uint:
		item.V = v - uint(n)
	case uintptr:
		item.V = v - uintptr(n)
	case uint8:
		item.V = v - uint8(n)
	case uint16:
		item.V = v - uint16(n)
	case uint32:
		item.V = v - uint32(n)
	case uint64:
		item.V = v - uint64(n)
	case float32:
		item.V = v - float32(n)
	case float64:
		item.V = v - float64(n)
	default:
		return fmt.Errorf("The value for %s is not an integer or float\n", k)
	}
	return nil
}

// DecrementFloat for float, item.V - n.
func (c *cache) DecrementFloat(k string, n float64) error {
	item, found := c.get4update(k)
	if !found {
		return fmt.Errorf("Item %s not found\n", k)
	}
	V := item.V
	switch v := V.(type) {
	case float64:
		item.V = v - n
	case float32:
		item.V = v - float32(n)
	default:
		return fmt.Errorf("The value for %s does not have type float32 or float64\n", k)
	}
	return nil
}

// ItemCount return count(item).
func (c *cache) ItemCount() (count int) {
	for i, n := 0, len(c.database); i < n; i++ {
		c.database[i].RLock()
		count += len(c.database[i].m)
		c.database[i].RUnlock()
	}
	return
}

// RunJanitor If you set CleanupInterval or SavingInterval, you do not need to call it explicitly.
// Of course, you can use this method to start a custom Janitor after mounting the Cache instance.
// Note that jt is a janitorType and needs to reference the Janitor constant(Cleanup or Saving).
func (c *cache) RunJanitor(jt janitorType) {
	switch jt {
	case Cleanup:
		go c.CleanupJanitor.run(c)
	case Saving:
		go c.SavingJanitor.run(c)
	}
}

// StopJanitor If you set CleanupInterval or SavingInterval, you do not need to call it explicitly.
// Of course, you can use this method to stop a custom Janitor after mounting the Cache instance.
// Note that jt is a janitorType and needs to reference the Janitor constant(Cleanup or Saving).
func (c *cache) StopJanitor(jt janitorType) {
	switch jt {
	case Cleanup:
		go c.CleanupJanitor.stop()
	case Saving:
		go c.SavingJanitor.stop()
	}
}

// OnExited supports configuring the actions you want to do when the program exits or crashes.
// And also supports choosing whether to export the data in the cache after your actions.
func (c *cache) OnExited(f func(), isSave bool) {
	c.onExited = f
	go c.onExit(isSave)
}

// OnEvicted supports configuring the actions you want to do after Delete || UnsafeDelete || c.clean().
func (c *cache) OnEvicted(f func(key string, value any)) { c.onEvicted = f }

// CacheSize return cache memory size.
func (c *cache) CacheSize() int { return c.nowBytes }

// GetShardMap return key's SharedMap.
// It is not recommended to directly operate SharedMap.
func (c *cache) GetShardMap(key string) *SharedMap {
	return c.database[uint(c.fnv32(key))%uint(c.shardCount)]
}

func (c *cache) fnv32(key string) uint32 {
	hash := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}

func (c *cache) clean() {
	root := c.ll.Back()
	ele := c.ll.Back()
	for ele != root {
		k := ele.Value.(string)
		sharedMap := c.GetShardMap(k)
		sharedMap.RLock()

		v, found := sharedMap.m[k]
		sharedMap.RUnlock()

		if found {
			item := v.Load().(*Item)
			if item.IsDeleted || (time.Now().UnixNano() > item.Expiration && item.Expiration != 0) {
				sizeBefore := int(unsafe.Sizeof(sharedMap))
				sharedMap.Lock()

				delete(sharedMap.m, k)
				sharedMap.Unlock()

				c.onEvicted(k, item.V)
				sizeAfter := int(unsafe.Sizeof(sharedMap))
				c.nowBytes -= sizeBefore - sizeAfter
			}
		}
		c.ll.Remove(ele)
		ele = c.ll.Back()
	}
}

func (c *cache) save() {
	items := c.Items()
	fileName := time.Now().Format("2006_01_02_15_04_05_000") + ".gob"
	filePath := filepath.Join("data", fileName)
	err := os.MkdirAll("data", os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create dataDirectory: %v", err)
	}
	fp, err := os.Create(filePath)
	if err != nil {
		_ = fp.Close()
		panic("Error create savingFile")
	}
	encoder := gob.NewEncoder(fp)
	err = encoder.Encode(&items)
	if err != nil {
		log.Fatalf("Failed to encode items: %v", err)
	}
	_ = fp.Close()
}

func (c *cache) onExit(isSave bool) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	for {
		select {
		case <-sigChan:
			c.onExited()
			if isSave {
				c.save()
			}
			return
		}
	}
}

func (c *cache) calExpiration(d time.Duration) int64 {
	var e int64
	switch d {
	case DefaultExpiration:
		d = DefaultExpiration
	case c.defaultExpiration:
		d = c.defaultExpiration
	default:
		e = time.Now().Add(d).UnixNano()
	}
	return e
}

func (c *cache) get4Get(k string) (*Item, bool) {
	sharedMap := c.GetShardMap(k)
	sharedMap.RLock()

	v, found := sharedMap.m[k]
	if !found {
		sharedMap.RUnlock()
		return nil, false
	}

	item, ok := v.Load().(*Item)
	if !ok {
		sharedMap.RUnlock()
		return nil, false
	}

	c.ll.MoveToFront(item.Ele)
	if item.IsDeleted {
		sharedMap.RUnlock()
		return item, false
	}

	if item.Expiration > 0 && time.Now().UnixNano() > item.Expiration {
		sharedMap.RUnlock()
		return nil, false
	}

	sharedMap.RUnlock()
	return item, true
}

func (c *cache) get4update(k string) (*Item, bool) {
	sharedMap := c.GetShardMap(k)
	sharedMap.RLock()

	v, found := sharedMap.m[k]
	if !found {
		sharedMap.RUnlock()
		return nil, false
	}

	item, ok := v.Load().(*Item)
	if !ok {
		sharedMap.RUnlock()
		return nil, false
	}

	c.ll.MoveToFront(item.Ele)
	sharedMap.RUnlock()

	return item, true
}

func (c *cache) updateUpdateTime(v *Item) { v.UpdateTime = time.Now().UnixNano() }

func (c *Cache) finalizer() {
	if c.CleanupJanitor != nil {
		c.CleanupJanitor.stop()
	}
	if c.SavingJanitor != nil {
		c.SavingJanitor.stop()
	}
}
