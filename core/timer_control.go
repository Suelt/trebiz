package core

import (
	"fmt"
	"time"
)

// softReset tells the timer to start a new countdown, only if it is not currently counting down
// this will not clear any pending events
func (et *timerImpl) SoftReset(timeout time.Duration, event interface{}) {
	et.startChan <- &timerStart{
		duration: timeout,
		event:    event,
		hard:     false,
	}
}

// reset tells the timer to start counting down from a new timeout, this also clears any pending events
func (et *timerImpl) Reset(timeout time.Duration, event interface{}) {
	et.startChan <- &timerStart{
		duration: timeout,
		event:    event,
		hard:     true,
	}
}

// stop tells the timer to stop, and not to deliver any pending events
func (et *timerImpl) Stop() {
	et.stopChan <- struct{}{}
}

// halt tells the threaded object's thread to exit
func (t *threaded) Halt() {
	select {
	case <-t.exit:
		//logger.Warning("Attempted to halt a threaded object twice")
	default:
		close(t.exit)
	}
}

type Timer interface {
	SoftReset(duration time.Duration, event interface{}) // start a new countdown, only if one is not already started
	Reset(duration time.Duration, event interface{})     // start a new countdown, clear any pending events
	Stop()                                               // stop the countdown, clear any pending events
	Halt()                                               // Stops the Timer thread
}

// threaded holds an exit channel to allow threads to break from a select
type threaded struct {
	exit chan struct{}
}

// TimerFactory abstracts the creation of Timers, as they may
// need to be mocked for testing
type TimerFactory interface {
	CreateTimer() Timer // Creates an Timer which is stopped
}

// CreateTimer creates a new timer which deliver events to the Manager for this factory
func (n *Node) CreateTimer(trans *NetworkTransport) Timer {
	return newTimerImpl(trans)
}

// timerStart is used to deliver the start request to the eventTimer thread
type timerStart struct {
	hard     bool          // Whether to reset the timer if it is running
	event    interface{}   // What event to push onto the event queue
	duration time.Duration // How long to wait before sending the event
}

// timerImpl is an implementation of Timer
type timerImpl struct {
	threaded                       // Gives us the exit chan
	timerChan    <-chan time.Time  // When non-nil, counts down to preparing to do the event
	startChan    chan *timerStart  // Channel to deliver the timer start events to the service go routine
	stopChan     chan struct{}     // Channel to deliver the timer stop events to the service go routine
	EventManager *NetworkTransport // The replica need to  deliver the event to after timer expiration
}

// newTimer creates a new instance of timerImpl
func newTimerImpl(trans *NetworkTransport) Timer {
	et := &timerImpl{
		startChan:    make(chan *timerStart),
		stopChan:     make(chan struct{}),
		threaded:     threaded{make(chan struct{})},
		EventManager: trans,
	}
	go et.loop()
	return et
}

// loop is where the timer thread lives, looping
func (et *timerImpl) loop() {

	//	var event interface{}
	for {
		// A little state machine, relying on the fact that nil channels will block on read/write indefinitely
		select {
		case start := <-et.startChan:
			if et.timerChan != nil {
				if start.hard {
					fmt.Println("Resetting a running timer")
				} else {
					continue
				}
			}
			fmt.Println("Starting timer")
			et.timerChan = time.After(start.duration)
		//	event=start.event

		case <-et.stopChan:
			if et.timerChan == nil {
				fmt.Println("Attempting to stop an unfired idle timer")
			}
			et.timerChan = nil
			fmt.Println("Stopping timer")
			//  event=nil

		case <-et.timerChan:
			fmt.Println("Event timer fired")
			et.timerChan = nil
			rpc := MsgWithSig{}
			rpc.Command = &ViewChangeTimerEvent{}
			select {
			case et.EventManager.consumeCh <- rpc:
				fmt.Println("Send viewChange")
			default:
				fmt.Println("Can't send viewChange")
			}
		case <-et.exit:
			fmt.Println("Halting timer")
			return
		}
	}
}
