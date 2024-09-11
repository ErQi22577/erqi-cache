# erqi-cache

## Overview
A cache reconstructed and optimized based on [go-cache](https://github.com/patrickmn/go-cache). It uses sharding and atomic operations to reduce the scope of lock usage and improve concurrency. At the same time, it expands the original cleanupJanitor cleaning conditions (Maxbytes) and cleaning methods (LRU), adds an automatically saved janitor, and supports dynamic mounting of janitors. In addition, the OnExited method is added to ensure cache data security.

## Installation
`go get github.com/ErQi22577/erqi-cache`

## Usage

### Create Cache with Config
```go
package main

import (
	"fmt"
	"github.com/ErQi22577/erqi-cache"
	"time"
)

func main() {
	c := cache.New(chache.Config{
		DefaultExpiration: 0, // Never expire.
		CleanupInterval:   0, // Disable cleanupJanitor.
		SavingInterval:    0, // Disable cleanupJanitor.
		ShardCount:        0, // Number of shard maps(0 mean that default 32). 
		MaxBytes:          0, // Max use memory(0 mean that default 16G).
	})
	
	err := c.Add("key", "value", 0, false) // false Disable overwrite.
	if err != nil {
		fmt.Println(err)
    }
	val, found := c.Get("key1")
	if !found {
		fmt.Println("Not Found.")
    }
}
```

### Create Cache with Items and Use Janitor

```go
package main

import (
	"fmt"
	"github.com/ErQi22577/erqi-cache"
	"time"
)

func main() {
	// First, items must exist.
	C := cache.New(chache.Config{})
	_ = C.Add("key", "value", 0, false)
	items := C.Items()

	// Set the cleanup interval and save interval, 
	// and you will get the corresponding janitor.
	c := cache.NewFrom(chache.Config{
		CleanupInterval: time.Duration(14400) * time.Minute,
		SavingInterval:  time.Duration(14400) * time.Minute,
	}, items)

	// Of course, you can also choose to mount the corresponding janitor at any time.
	C.CleanupJanitor = &cache.Janitor{
		Interval: time.Duration(14400) * time.Minute,
		StopChan: make(chan bool),
		Type: cache.Cleanup,
	}
	C.RunJanitor(cache.Cleanup)
	C.StopJanitor(cache.Cleanup)
}
```
For more usage, see cache_test.

## Methods
    
### Add
```go
Add(k string, x any, d time.Duration, overWrite bool) error
```
k key, x value, d defaultExpiration(0:Never expire.).

### Delete
```go
Delete(k string)
```
really remove item.

### UnsafeDelete
```go
UnsafeDelete(k string)
```
Only modifies item.IsDeleted to make it invisible.

### Update
```go
Update(k string, x any) error
```
Alter item.V if item can be found.

### UpdateExpiration
```go
UpdateExpiration(k string, d time.Duration) error
```
Alter item.ExpirationTime if item can be found.

### Get
```go
Get(k string) (any, bool)
```
Return item.V, isFound. 
If item has expired or isDeleted is true, will not be found.

### UnsafeGet
```go
UnsafeGet(k string) (any, bool)
```
Return item.V, isExist.
It will also return data that has expired and isDeleted is true.

### GetWithExpiration
```go
GetWithExpiration(k string) (any, time.Time, bool)
```
Return item.V, item.ExpirationTime, isFound.

### GetCreateTime
```go
GetCreateTime(k string) (time.Time, bool)
```
Return item.CreateTime, isFound.

### GetUpdateTime
```go
GetUpdateTime(k string) (time.Time, bool)
```
Return item.UpdateTime, isFound.

### Increment
```go
Increment(k string, n int64) error
```
For int, item.V + n.

### IncrementFloat
```go
IncrementFloat(k string, n float64) error
```
For float, item.V + n.

### Decrement
```go
Decrement(k string, n int64) error
```
For int, item.V - n.

### DecrementFloat
```go
DecrementFloat(k string, n float64) error
```
For float, item.V - n.

### ItemCount
```go
ItemCount() (count int)
```
Return count(item).

### RunJanitor
```go
RunJanitor(jt janitorType)
```
If you set CleanupInterval or SavingInterval, you do not need to call it explicitly.
Of course, you can use this method to start a custom Janitor after mounting the Cache instance.
Note that jt is a janitorType and needs to reference the Janitor constant(Cleanup or Saving).

### StopJanitor
```go
StopJanitor(jt janitorType)
```
If you set CleanupInterval or SavingInterval, you do not need to call it explicitly.
Of course, you can use this method to stop a custom Janitor after mounting the Cache instance.
Note that jt is a janitorType and needs to reference the Janitor constant(Cleanup or Saving).

### OnExited
```go
OnExited(f func(), isSave bool)
```
Supports configuring the actions you want to do when the program exits or crashes.
And also supports choosing whether to export the data in the cache after your actions.

### OnEvicted
```go
OnEvicted(f func(key string, value any))
```
Supports configuring the actions you want to do after Delete || UnsafeDelete || c.clean().

### CacheSize
```go
CacheSize() int
```
Return cache memory size.

### GetShardMap
```go
GetShardMap(key string) *SharedMap
```
Return key's SharedMap.
It is not recommended to directly operate SharedMap.

### Items
```go
Items() map[string]Item
```
Exports all data in the cache for data persistence or data migration.

### ItemsInit
```go
ItemsInit(items map[string]Item)
```
Writes the data exported by the c.Items() into the cache.

### Flush
```go
Flush()
```
Clear all data in the cache.

### NewFrom
```go
NewFrom(config Config, items map[string]Item) *Cache
```
Encapsulates the New and c.ItemInit(). You can also use them separately.
Note that the items parameter needs to be generated in advance using the c.Items().
It is not recommended to pass custom items directly, as this may cause unknown errors.

### New
```go
New(config Config) *Cache
```
Create Cache with Config

## Author
-**ErQi**-_Full Stack Developer_-[ErQi22577](https://github.com/ErQi22577)
