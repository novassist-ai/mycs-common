package utils

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/novassist/mycs-common/pkg/goutils/logger"
)

type TaskDispatcher struct {
	putTimeout    time.Duration
	dispatchQueue chan *task

	queued sync.WaitGroup
	stop   int32
}

type task struct {
	td *TaskDispatcher

	name     string
	numTries int

	taskData interface{}

	taskFn    func(inData interface{}) (outData interface{}, err error)
	successFn func(outData interface{})
	errorFn   func(err error, outData interface{})
}

func NewTaskDispatcher(bufferSize int, putTimeout time.Duration) *TaskDispatcher {

	return &TaskDispatcher{
		putTimeout:    putTimeout,
		dispatchQueue: make(chan *task, bufferSize),

		stop: 0,
	}
}

func (td *TaskDispatcher) Start(numWorkers int) {

	for i := 0; i < numWorkers; i++ {
		workerIndex := i
		
		go func() {
			logger.TraceMessage("TaskDispatcher.Worker(): Running task worker '%d'.", workerIndex)

			var (
				err error

				outData interface{}
			)

			for t := range td.dispatchQueue {
				logger.TraceMessage(
					"TaskDispatcher.Worker(): Running task '%s' with data: %# v",
					t.name, t.taskData,
				)
		
				if outData, err = t.taskFn(t.taskData); err != nil {

					if atomic.LoadInt32(&td.stop) == 1 || t.numTries == 0 || outData == nil {
						// if no more retries and still 
						// having errors then inform the 
						// task owner with the error and 
						// remaining unprocessed data
						if t.errorFn != nil {
							t.errorFn(err, outData)
						} else {
							logger.TraceMessage(
								"TaskDispatcher.Worker(): Task '%s' execution failed with output data: %# v",
								t.name, outData,
							)
							logger.ErrorMessage(
								"TaskDispatcher.Worker(): Task '%s' execution failed with error: %s",
								t.name, err.Error(),
							)
						}
						
					} else {
						logger.TraceMessage(
							"TaskDispatcher.Worker(): Task '%s' execution failed with output data: %# v",
							t.name, outData,
						)
						logger.ErrorMessage(
							"TaskDispatcher.Worker(): Task '%s' execution failed with error: %s; Will retry '%d' more times.",
							t.name, err.Error(), t.numTries,
						)

						// rerun task fn until task 
						// completes or the retry counter 
						// is done
						t.numTries--
						t.taskData = outData						
						if dispatchErr := t.td.dispatch(t); dispatchErr != nil {
							logger.ErrorMessage("TaskDispatcher.Worker(): %s.", dispatchErr.Error())
							if t.errorFn != nil {
								t.errorFn(err, outData)
							}							
						}
					}

				} else {
					if (t.successFn != nil) {
						t.successFn(outData)
					} else {
						logger.TraceMessage(
							"TaskDispatcher.Worker(): Task '%s' execution succeeded with output data %# v",
							t.name, outData,
						)
					}
				}
				
				td.queued.Done()
			}

			logger.TraceMessage("TaskDispatcher.Worker(): Task worker '%d' done.", workerIndex)
		}()
	}
}

func (td *TaskDispatcher) Stop() {	
	// signal to stop retrying
	atomic.StoreInt32(&td.stop, 1)
	// wait for all tasks to complete or error out
	td.queued.Wait()

	close(td.dispatchQueue)
}

func (td *TaskDispatcher) RunTask(
	name string,
	taskFn func(inData interface{}) (outData interface{}, err error), 
) *task {

	return &task{
		td:     td,
		name:   name,
		taskFn: taskFn,
	}
}

func (td *TaskDispatcher) dispatch(t *task) error {
	td.queued.Add(1)
	select {
	case td.dispatchQueue <- t:
		return nil
	case <- time.After(td.putTimeout * time.Millisecond):
		td.queued.Done()
		return fmt.Errorf("timed out attempting to dispatch task %s", t.name)
	}
}

func (t *task) WithData(taskData interface{}) *task {
	t.taskData = taskData
	return t
}

func (t *task) OnSuccess(successFn func(outData interface{})) *task {
	t.successFn = successFn
	return t
}

func (t *task) OnError(errorFn func(err error, unprocessedData interface{})) *task {
	t.errorFn = errorFn
	return t
}

func (t *task) Once() error {	
	return t.td.dispatch((t))
}

func (t *task) WithRetries(numRetries int) error {
	t.numTries = numRetries
	return t.td.dispatch((t))
}
