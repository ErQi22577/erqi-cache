package erqi_cache

import (
	"time"
)

const (
	Cleanup janitorType = "CLEANUP"
	Saving  janitorType = "SAVING"
)

type janitorType string

type Janitor struct {
	Interval time.Duration
	StopChan chan bool
	Type     janitorType
}

func (j *Janitor) run(c *cache) {
	overSizeChan := make(chan bool)
	ticker := time.NewTicker(j.Interval)

	switch j.Type {
	case Cleanup:
		for {
			if c.nowBytes >= c.maxBytes {
				overSizeChan <- true
			}
			select {
			case <-ticker.C:
				c.clean()
			case <-overSizeChan:
				c.clean()
			case <-j.StopChan:
				ticker.Stop()
				return
			}
		}
	case Saving:
		for {
			select {
			case <-ticker.C:
				c.save()
			case <-j.StopChan:
				ticker.Stop()
				return
			}
		}
	}
}

func (j *Janitor) stop() { j.StopChan <- true }
