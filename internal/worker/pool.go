package worker

import (
	"context"
	"fmt"
	"opertraitor/internal/ui"
	"sync"
	"time"
)

// ProcessQueue runs producer/consumer work across a bounded goroutine pool.
func ProcessQueue(ctx context.Context, count int, workerCount int, jobTimeout time.Duration, producer func(chan<- interface{}), consumer func(context.Context, interface{})) {
	jobs := make(chan interface{}, count)
	var wg sync.WaitGroup

	if workerCount < 1 {
		workerCount = 1
	}
	workers := workerCount
	if count < workers && count > 0 {
		workers = count
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					func() {
						defer func() {
							if r := recover(); r != nil {
								ui.LogError(fmt.Sprintf("Worker recovered from panic: %v", r))
							}
						}()
						jobCtx, cancel := context.WithTimeout(ctx, jobTimeout)
						defer cancel()
						consumer(jobCtx, job)
					}()
				}
			}
		}()
	}

	producer(jobs)
	close(jobs)
	wg.Wait()
}
