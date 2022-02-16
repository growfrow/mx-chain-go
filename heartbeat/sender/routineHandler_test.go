package sender

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ElrondNetwork/elrond-go/heartbeat/mock"
	"github.com/stretchr/testify/assert"
)

func TestRoutineHandler_ShouldWork(t *testing.T) {
	t.Parallel()

	t.Run("should work concurrently, calling both handlers, twice", func(t *testing.T) {
		t.Parallel()

		ch1 := make(chan time.Time)
		ch2 := make(chan time.Time)

		numExecuteCalled1 := uint32(0)
		numExecuteCalled2 := uint32(0)

		handler1 := &mock.SenderHandlerStub{
			ExecutionReadyChannelCalled: func() <-chan time.Time {
				return ch1
			},
			ExecuteCalled: func() {
				atomic.AddUint32(&numExecuteCalled1, 1)
			},
		}
		handler2 := &mock.SenderHandlerStub{
			ExecutionReadyChannelCalled: func() <-chan time.Time {
				return ch2
			},
			ExecuteCalled: func() {
				atomic.AddUint32(&numExecuteCalled2, 1)
			},
		}

		_ = newRoutineHandler(handler1, handler2)
		time.Sleep(time.Second) // wait for the go routine start

		assert.Equal(t, uint32(1), atomic.LoadUint32(&numExecuteCalled1)) // initial call
		assert.Equal(t, uint32(1), atomic.LoadUint32(&numExecuteCalled2)) // initial call

		go func() {
			time.Sleep(time.Millisecond * 100)
			ch1 <- time.Now()
		}()
		go func() {
			time.Sleep(time.Millisecond * 100)
			ch2 <- time.Now()
		}()

		time.Sleep(time.Second) // wait for the iteration

		assert.Equal(t, uint32(2), atomic.LoadUint32(&numExecuteCalled1))
		assert.Equal(t, uint32(2), atomic.LoadUint32(&numExecuteCalled2))
	})
	t.Run("close should work", func(t *testing.T) {
		t.Parallel()

		ch1 := make(chan time.Time)
		ch2 := make(chan time.Time)

		numExecuteCalled1 := uint32(0)
		numExecuteCalled2 := uint32(0)

		numCloseCalled1 := uint32(0)
		numCloseCalled2 := uint32(0)

		handler1 := &mock.SenderHandlerStub{
			ExecutionReadyChannelCalled: func() <-chan time.Time {
				return ch1
			},
			ExecuteCalled: func() {
				atomic.AddUint32(&numExecuteCalled1, 1)
			},
			CloseCalled: func() {
				atomic.AddUint32(&numCloseCalled1, 1)
			},
		}
		handler2 := &mock.SenderHandlerStub{
			ExecutionReadyChannelCalled: func() <-chan time.Time {
				return ch2
			},
			ExecuteCalled: func() {
				atomic.AddUint32(&numExecuteCalled2, 1)
			},
			CloseCalled: func() {
				atomic.AddUint32(&numCloseCalled2, 1)
			},
		}

		rh := newRoutineHandler(handler1, handler2)
		time.Sleep(time.Second) // wait for the go routine start

		assert.Equal(t, uint32(1), atomic.LoadUint32(&numExecuteCalled1)) // initial call
		assert.Equal(t, uint32(1), atomic.LoadUint32(&numExecuteCalled2)) // initial call
		assert.Equal(t, uint32(0), atomic.LoadUint32(&numCloseCalled1))
		assert.Equal(t, uint32(0), atomic.LoadUint32(&numCloseCalled2))

		rh.closeProcessLoop()

		time.Sleep(time.Second) // wait for the go routine to stop

		assert.Equal(t, uint32(1), atomic.LoadUint32(&numExecuteCalled1))
		assert.Equal(t, uint32(1), atomic.LoadUint32(&numExecuteCalled2))
		assert.Equal(t, uint32(1), atomic.LoadUint32(&numCloseCalled1))
		assert.Equal(t, uint32(1), atomic.LoadUint32(&numCloseCalled2))
	})
}
