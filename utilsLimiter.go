package main

import "sync"

type Limiter struct {
	sem  chan struct{}
	once sync.Once
}

func NewLimiter(limits int) *Limiter {
	return &Limiter{
		sem: make(chan struct{}, limits),
	}
}

func (l *Limiter) Acquire() chan<- struct{} {
	return l.sem
}

func (l *Limiter) Release() {
	<-l.sem
}

func (l *Limiter) Close() {
	l.once.Do(func() {
		close(l.sem)
	})
}
