// Package scheduler implements a scheduler using the time wheel algorithm
package scheduler

import (
	"container/list"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type SchedulerOpts func(*TimeWheelScheduler) *TimeWheelScheduler

// Job is the unit of work to be done/scheduled/executed
type Job struct {
	id         uuid.UUID
	jobType    string
	payload    map[string]any
	expiration int64
	element    *list.Element
	bucket     *Bucket
}

// Bucket hold a collection of jobs in the same range e.g hours bucket, minutes bucket
// It holds a pointer to the first item on the execution list and a mutex
type Bucket struct {
	jobs *list.List
}

// Wheel hold a slice of buckets e.g. hour bucket, minute bucket
// size is the max number of buckets available in this Wheel
// interval is the takes to cover the wheel. e.g for 1 minute is 60 intervals
// lower is the closest smaller bucket e.g. for minutes bucket, the lower bucket is the seconds bucket
type Wheel struct {
	buckets  []*Bucket
	size     int64
	interval int64
	position int64
	lower    *Wheel
	upper    *Wheel
}

// TimeWheelScheduler holds all the wheels, a ticker and a channel to send a stop signal for graceful shutdown
type TimeWheelScheduler struct {
	hours   *Wheel
	minutes *Wheel
	seconds *Wheel

	tick *time.Ticker
	stop chan struct{}
}

func NewJob(jobType string, payload map[string]any, expiration time.Duration) *Job {
	after := time.Now().UTC().Add(expiration).UnixNano()
	return &Job{id: uuid.New(), jobType: jobType, expiration: after, payload: payload}
}

func NewBucket() *Bucket {
	return &Bucket{jobs: list.New()}
}

func (b *Bucket) AddJob(j *Job) {
	node := b.jobs.PushBack(j)
	j.element = node
	j.bucket = b
}

func (b *Bucket) Flush(f func(j *Job)) {
	for j := b.jobs.Front(); j != nil; {
		next := j.Next()
		job := j.Value.(*Job)
		b.jobs.Remove(j)
		job.element = nil
		job.bucket = nil
		f(job)
		j = next
	}
}

func (b *Bucket) CancelJob(j *Job) {
	b.jobs.Remove(j.element)
	j.element = nil
	j.bucket = nil
}

func NewWheel(size int64, interval time.Duration) *Wheel {
	wheel := &Wheel{
		buckets:  make([]*Bucket, size),
		size:     size,
		interval: int64(interval),
	}

	for i := range size {
		wheel.buckets[i] = NewBucket()
	}

	return wheel
}

func (w *Wheel) AddJob(j *Job) {
	pos := calculateBucketIdx(w.position, w.interval, w.size, j.expiration)

	log.Printf("inserting job %s to position %d: ", j.id, pos)

	w.buckets[pos].AddJob(j)
}

func NewTimeWheelScheduler(ticker *time.Ticker, stop chan struct{}, opts ...SchedulerOpts) *TimeWheelScheduler {
	scheduler := &TimeWheelScheduler{
		tick: ticker,
		stop: stop,
	}

	for _, opt := range opts {
		opt(scheduler)
	}

	return scheduler
}

func (tw *TimeWheelScheduler) WithHoursWheel(wheel *Wheel) *TimeWheelScheduler {
	tw.hours = wheel
	return tw
}

func (tw *TimeWheelScheduler) WithMinutesWheel(wheel *Wheel) *TimeWheelScheduler {
	tw.minutes = wheel
	return tw
}

func (tw *TimeWheelScheduler) WithSecondsWheel(wheel *Wheel) *TimeWheelScheduler {
	tw.seconds = wheel
	return tw
}

func (tw *TimeWheelScheduler) AddJob(job *Job) error {
	if job == nil {
		return errors.New("job cannot be nil")
	}

	now := time.Now().UTC().UnixNano()
	diff := job.expiration - now

	switch {
	case diff < int64(time.Minute):
		log.Println("Job added to seconds bucket")
		tw.seconds.AddJob(job)
	case diff < int64(time.Hour):
		log.Println("Job added to minutes bucket")
		tw.minutes.AddJob(job)
	default:
		log.Println("Job added to hours bucket")
		tw.hours.AddJob(job)
	}

	return nil
}

func (tw *TimeWheelScheduler) Tick(wheel *Wheel) {
	pos := wheel.position
	bucket := wheel.buckets[pos]

	log.Println("Processing bucket: ", bucket)

	bucket.Flush(func(j *Job) {
		// Checks if we're in the seconds bucket
		// If not, we cascade the job/reassign
		if wheel.lower == nil {
			go func(j *Job) {
				fmt.Println("Executed job ID: ", j.id.ID())
			}(j)
		} else {
			tw.AddJob(j)
		}
	})

	wheel.position = (pos + 1) % wheel.size

	if wheel.position == 0 && wheel.upper != nil {
		tw.Tick(wheel.upper)
	}
}

func (tw *TimeWheelScheduler) Start() {
	for {
		select {
		case <-tw.tick.C:
			tw.Tick(tw.seconds)
		default:
			return
		}
	}
}

func calculateBucketIdx(position, interval, size, expiration int64) int64 {
	now := time.Now().UTC().UnixNano()
	diff := expiration - now

	ticks := diff / interval
	pos := (position + ticks) % size
	return pos
}
