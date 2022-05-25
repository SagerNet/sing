package task

import (
	"context"
	"sync"

	"github.com/sagernet/sing/common"
)

func Run(ctx context.Context, tasks ...func() error) error {
	ctx, cancel := context.WithCancel(ctx)
	wg := &sync.WaitGroup{}
	wg.Add(len(tasks))
	var retErr error
	for _, task := range tasks {
		task := task
		go func() {
			if err := task(); err != nil {
				if !common.Done(ctx) {
					retErr = err
				}
				cancel()
			}
			wg.Done()
		}()
	}
	go func() {
		wg.Wait()
		cancel()
	}()
	<-ctx.Done()
	return retErr
}
