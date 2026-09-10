package utils

import (
	"context"
	"sync"
)

func Await(tasks ...func(context.Context)) error {
	return AwaitCtx(context.Background(), tasks...)
}

func AwaitCtx(ctx context.Context, tasks ...func(context.Context)) error {
	wg := &sync.WaitGroup{}
	done := make(chan struct{})

	for _, task := range tasks {
		wg.Go(func() { task(ctx) })
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
