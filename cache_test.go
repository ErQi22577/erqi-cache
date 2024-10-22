package cache

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCache_New(t *testing.T) {
	t.Run("New cache with config.", func(t *testing.T) {
		c := New(Config{
			DefaultExpiration: time.Duration(0) * time.Second,
			CleanupInterval:   time.Duration(14400) * time.Minute,
			SavingInterval:    time.Duration(14400) * time.Minute,
			ShardCount:        0,
			MaxBytes:          0,
		})
		fmt.Println(len(c.database))
		c = nil
	})
}

func TestCache_NewFrom(t *testing.T) {
	t.Run("New cache from items.", func(t *testing.T) {
		c := NewFrom(Config{
			DefaultExpiration: time.Duration(0) * time.Second,
			CleanupInterval:   time.Duration(14400) * time.Minute,
			SavingInterval:    time.Duration(14400) * time.Minute,
			ShardCount:        0,
			MaxBytes:          0,
		}, getTestData())
		fmt.Println(c.Get("test"))
		c = nil
	})
}

func TestCache_Flush(t *testing.T) {
	t.Run("Flush cache data.", func(t *testing.T) {
		c := getTestCache()
		c.Flush()
		fmt.Println(c.Get("test"))
		c = nil
	})
}

func TestCache_Add(t *testing.T) {
	t.Run("Add data to cache.", func(t *testing.T) {
		c := getTestCache()
		_ = c.Add("test1", "ok1", time.Duration(1)*time.Hour)
		err := c.Add("test1", "ok2", time.Duration(1)*time.Hour)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(c.Get("test"))
		c = nil
	})
}

func TestCache_Set(t *testing.T) {
	t.Run("Set data to cache.(overwrite)", func(t *testing.T) {
		c := getTestCache()
		_ = c.Set("test1", "ok1", time.Duration(1)*time.Hour)
		err := c.Set("test1", "ok2", time.Duration(1)*time.Hour)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(c.Get("test"))
		c = nil
	})
}

func TestCache_Delete(t *testing.T) {
	t.Run("Delete data from cache.", func(t *testing.T) {
		c := getTestCache()
		c.Delete("test")
		fmt.Println(c.Get("test"))
		c = nil
	})

	t.Run("Delete data from cache.(unsafe)", func(t *testing.T) {
		c := getTestCache()
		c.UnsafeDelete("test")
		fmt.Println(c.Get("test"))
		fmt.Println(c.UnsafeGet("test"))
		c = nil
	})
}

func TestCache_Update(t *testing.T) {
	t.Run("Update data from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.Get("test"))
		_ = c.Update("test", "ok2")
		fmt.Println(c.Get("test"))
		c = nil
	})

	t.Run("Update data's Expiration from cache.", func(t *testing.T) {
		c := getTestCache()
		_ = c.UpdateExpiration("test", time.Duration(60)*time.Second)
		fmt.Println(c.GetWithExpiration("test"))
		c = nil
	})
}

func TestCache_Get(t *testing.T) {
	t.Run("Get data from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.Get("test"))
		c = nil
	})

	t.Run("Get data from cache.(unsafe)", func(t *testing.T) {
		c := getTestCache()
		c.UnsafeDelete("test")
		fmt.Println(c.Get("test"))
		fmt.Println(c.UnsafeGet("test"))
		c = nil
	})

	t.Run("Get data with expiration from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.GetWithExpiration("test"))
		c = nil
	})

	t.Run("Get data's CreateTime from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.GetCreateTime("test"))
		c = nil
	})

	t.Run("Get data's UpdateTime from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.GetUpdateTime("test"))
		c = nil
	})
}

func TestCache_Increment_Decrement(t *testing.T) {
	t.Run("Increment data from cache.(int)", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.Get("test_int"))
		_ = c.Increment("test_int", 2)
		fmt.Println(c.Get("test_int"))
		c = nil
	})

	t.Run("Decrement data from cache.(int)", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.Get("test_int"))
		_ = c.Decrement("test_int", 2)
		fmt.Println(c.Get("test_int"))
		c = nil
	})

	t.Run("IncrementFloat data from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.Get("test_float"))
		_ = c.IncrementFloat("test_float", 1.2)
		fmt.Println(c.Get("test_float"))
		c = nil
	})

	t.Run("DecrementFloat data from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.Get("test_float"))
		_ = c.DecrementFloat("test_float", 1.)
		fmt.Println(c.Get("test_float"))
		c = nil
	})
}

func TestCache_ItemCount_CacheSize(t *testing.T) {
	t.Run("Count data from cache.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.ItemCount())
		c = nil
	})

	t.Run("CalCacheSize (cache memory size).", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.CacheSize())
		c = nil
	})
}

func TestCache_RunJanitor_StopJanitor(t *testing.T) {
	t.Run("Custom run and stop janitor.", func(t *testing.T) {
		c := getTestCache()
		c.UnsafeDelete("test")
		fmt.Println(c.UnsafeGet("test"))
		c.SavingJanitor = &Janitor{
			Interval: time.Duration(1) * time.Second,
			StopChan: make(chan bool),
			Type:     Saving,
		}
		c.CleanupJanitor = &Janitor{
			Interval: time.Duration(1) * time.Second,
			StopChan: make(chan bool),
			Type:     Cleanup,
		}
		c.RunJanitor(Saving)
		c.RunJanitor(Cleanup)
		fmt.Println(c.UnsafeGet("test"))
		c.StopJanitor(Cleanup)
		c.StopJanitor(Saving)
		//time.Sleep(1 * time.Minute)
		c = nil
	})
}

func TestCache_OnEvicted_OnExited(t *testing.T) {
	t.Run("Custom OnEvicted and OnExited.", func(t *testing.T) {
		c := getTestCache()
		c.OnEvicted(func(key string, value any) {
			if key == "test" {
				_ = c.Add("TEST", "success", 0)
			}
		})
		c.OnExited(func() {
			_ = c.Add("TEST", "success", 0)
		}, true)
		fmt.Println(c.Get("TEST"))
		c.Delete("test")
		fmt.Println(c.Get("TEST"))
	})
}

func TestCache_GetShardMap(t *testing.T) {
	t.Run("Get key's GetShardMap.", func(t *testing.T) {
		c := getTestCache()
		fmt.Println(c.GetShardMap("test"))
		c = nil
	})
}

func getTestCache() *Cache {
	c := New(Config{
		DefaultExpiration: time.Duration(0) * time.Second,
		CleanupInterval:   time.Duration(0) * time.Second,
		SavingInterval:    time.Duration(0) * time.Second,
		ShardCount:        0,
		MaxBytes:          0,
	})
	_ = c.Add("test", "ok", 0)
	_ = c.Add("test_int", 1, 0)
	_ = c.Add("test_float", 1.1, 0)
	return c
}

func BenchmarkCache(b *testing.B) {
	c := New(Config{ShardCount: 64})
	b.StartTimer()
	var wg sync.WaitGroup
	wg.Add(1001)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			tmp := i
			go func() {
				defer wg.Done()
				_ = c.Add(strconv.Itoa(tmp), "abcdefghijklmnopqrstuvwxyz", 0)
			}()
		}
	}()
	wg.Wait()
	wg.Add(2002)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			tmp := i
			go func() {
				defer wg.Done()
				_ = c.Update(strconv.Itoa(tmp), "abcdefghijklmnopqrstuvw")
			}()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			tmp := i
			go func() {
				defer wg.Done()
				_, found := c.Get(strconv.Itoa(tmp))
				if found {
					// fmt.Println(i)
				}
			}()
		}
	}()
	wg.Wait()
	b.StopTimer()
}

//func BenchmarkGoCache(b *testing.B) {
//	c := goCache.New(time.Duration(0)*time.Second, time.Duration(0)*time.Second)
//	b.StartTimer()
//	var wg sync.WaitGroup
//	wg.Add(1001)
//	go func() {
//		defer wg.Done()
//		for i := 0; i < 1000; i++ {
//			go func(i int) {
//				defer wg.Done()
//				_ = c.Add(strconv.Itoa(i), "abcdefghijklmnopqrstuvwxyz", 0)
//			}(i)
//		}
//	}()
//	wg.Wait()
//	wg.Add(2002)
//
//	go func() {
//		defer wg.Done()
//		for i := 0; i < 1000; i++ {
//			go func(i int) {
//				defer wg.Done()
//				c.Set(strconv.Itoa(i), "abcdefghijklmnopqrstuvw", 0)
//			}(i)
//		}
//	}()
//	go func() {
//		defer wg.Done()
//		for i := 0; i < 1000; i++ {
//			go func(i int) {
//				defer wg.Done()
//				_, found := c.Get(strconv.Itoa(i))
//				if found {
//					//fmt.Println(i)
//				}
//			}(i)
//		}
//	}()
//	wg.Wait()
//	b.StopTimer()
//}

func getTestData() map[string]Item {
	c := New(Config{
		DefaultExpiration: time.Duration(0) * time.Second,
		CleanupInterval:   time.Duration(0) * time.Second,
		SavingInterval:    time.Duration(0) * time.Second,
		ShardCount:        2,
		MaxBytes:          0,
	})
	_ = c.Add("test", "ok", 0)
	return c.Items()
}
