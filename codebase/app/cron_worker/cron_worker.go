package cronworker

// cron scheduler worker, create with 100% pure internal go library (using reflect select channel)

import (
	"context"
	"fmt"
	"log"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/golangid/candi/candishared"
	"github.com/golangid/candi/codebase/factory"
	"github.com/golangid/candi/codebase/factory/types"
	"github.com/golangid/candi/logger"
	"github.com/golangid/candi/tracer"
)

type cronWorker struct {
	ctx           context.Context
	ctxCancelFunc func()

	opt       option
	service   factory.ServiceFactory
	wg        sync.WaitGroup
	semaphore map[string]chan struct{}
}

// NewWorker create new cron worker
func NewWorker(service factory.ServiceFactory, opts ...OptionFunc) factory.AppServerFactory {
	c := &cronWorker{
		service:   service,
		opt:       getDefaultOption(),
		semaphore: make(map[string]chan struct{}),
	}

	for _, opt := range opts {
		opt(&c.opt)
	}

	refreshWorkerNotif, shutdown = make(chan struct{}), make(chan struct{})
	startWorkerCh, releaseWorkerCh = make(chan struct{}), make(chan struct{})

	// add shutdown channel to first index
	workers = append(workers, reflect.SelectCase{
		Dir: reflect.SelectRecv, Chan: reflect.ValueOf(shutdown),
	})
	// add refresh worker channel to second index
	workers = append(workers, reflect.SelectCase{
		Dir: reflect.SelectRecv, Chan: reflect.ValueOf(refreshWorkerNotif),
	})

	c.opt.locker.Reset(fmt.Sprintf(lockPattern, c.service.Name(), "*"))
	for _, m := range service.GetModules() {
		if h := m.WorkerHandler(types.Scheduler); h != nil {
			var handlerGroup types.WorkerHandlerGroup
			h.MountHandlers(&handlerGroup)
			for _, handler := range handlerGroup.Handlers {
				funcName, args, interval := ParseCronJobKey(handler.Pattern)

				var job Job
				job.HandlerName = funcName
				job.Handler = handler
				job.Interval = interval
				job.Params = args
				if err := AddJob(job); err != nil {
					panic(fmt.Errorf(`Cron Worker: "%s" %v`, interval, err))
				}

				c.semaphore[funcName] = make(chan struct{}, c.opt.maxGoroutines)
				logger.LogYellow(fmt.Sprintf(`[CRON-WORKER] (job name): %s (every): %-8s  --> (module): "%s"`, `"`+funcName+`"`, interval, m.Name()))
			}
		}
	}
	fmt.Printf("\x1b[34;1m⇨ Cron worker running with %d jobs\x1b[0m\n\n", len(activeJobs))

	c.ctx, c.ctxCancelFunc = context.WithCancel(context.Background())
	return c
}

func (c *cronWorker) Serve() {
	c.createConsulSession()

START:
	select {
	case <-startWorkerCh:
		startAllJob()
		totalRunJobs := 0

		// run worker
		for {
			chosen, _, ok := reflect.Select(workers)
			if !ok {
				continue
			}

			// if shutdown channel captured, break loop (no more jobs will run)
			if chosen == 0 {
				return
			}

			// notify for refresh worker
			if chosen == 1 {
				continue
			}

			chosen = chosen - 2
			job := activeJobs[chosen]
			if job.nextDuration != nil {
				job.ticker.Stop()
				job.currentDuration = *job.nextDuration
				job.ticker = time.NewTicker(*job.nextDuration)
				workers[job.WorkerIndex].Chan = reflect.ValueOf(job.ticker.C)
				activeJobs[chosen].nextDuration = nil
			}

			if len(c.semaphore[job.HandlerName]) >= c.opt.maxGoroutines {
				continue
			}

			c.semaphore[job.HandlerName] <- struct{}{}
			c.wg.Add(1)
			go func(j *Job) {
				defer func() {
					c.wg.Done()
					<-c.semaphore[j.HandlerName]
				}()

				if c.ctx.Err() != nil {
					logger.LogRed("cron_scheduler > ctx root err: " + c.ctx.Err().Error())
					return
				}
				c.processJob(j)
			}(job)

			if c.opt.consul != nil {
				totalRunJobs++
				// if already running n jobs, release lock so that run in another instance
				if totalRunJobs == c.opt.consul.MaxJobRebalance {
					// recreate session
					c.createConsulSession()
					<-releaseWorkerCh
					goto START
				}
			}
		}

	case <-shutdown:
		return
	}
}

func (c *cronWorker) Shutdown(ctx context.Context) {
	defer func() {
		if c.opt.consul != nil {
			if err := c.opt.consul.DestroySession(); err != nil {
				panic(err)
			}
		}
		log.Println("\x1b[33;1mStopping Cron Job Scheduler:\x1b[0m \x1b[32;1mSUCCESS\x1b[0m")
	}()

	if len(activeJobs) == 0 {
		return
	}

	stopAllJob()
	shutdown <- struct{}{}
	runningJob := 0
	for _, sem := range c.semaphore {
		runningJob += len(sem)
	}
	if runningJob != 0 {
		fmt.Printf("\x1b[34;1mCron Job Scheduler:\x1b[0m waiting %d job until done...\n", runningJob)
	}

	c.wg.Wait()
	c.ctxCancelFunc()
}

func (c *cronWorker) Name() string {
	return string(types.Scheduler)
}

func (c *cronWorker) createConsulSession() {
	if c.opt.consul == nil {
		go func() { startWorkerCh <- struct{}{} }()
		return
	}
	c.opt.consul.DestroySession()
	stopAllJob()
	hostname, _ := os.Hostname()
	value := map[string]string{
		"hostname": hostname,
	}
	go c.opt.consul.RetryLockAcquire(value, startWorkerCh, releaseWorkerCh)
}

func (c *cronWorker) processJob(job *Job) {
	ctx := c.ctx
	if job.Handler.DisableTrace {
		ctx = tracer.SkipTraceContext(ctx)
	}

	// lock for multiple worker (if running on multiple pods/instance)
	if c.opt.locker.IsLocked(c.getLockKey(job.HandlerName)) {
		return
	}
	defer c.opt.locker.Unlock(c.getLockKey(job.HandlerName))

	trace, ctx := tracer.StartTraceWithContext(ctx, "CronScheduler")
	defer func() {
		if r := recover(); r != nil {
			trace.SetError(fmt.Errorf("%v", r))
		}
		logger.LogGreen("cron scheduler > trace_url: " + tracer.GetTraceURL(ctx))
		trace.Finish()
	}()
	trace.SetTag("job_name", job.HandlerName)
	trace.SetTag("job_param", job.Params)

	if c.opt.debugMode {
		log.Printf("\x1b[35;3mCron Scheduler: executing task '%s' (interval: %s)\x1b[0m", job.HandlerName, job.Interval)
	}

	var eventContext candishared.EventContext
	eventContext.SetContext(ctx)
	eventContext.SetWorkerType(string(types.Scheduler))
	eventContext.SetHandlerRoute(job.HandlerName)
	eventContext.SetHeader(map[string]string{
		"interval": job.Interval,
	})
	eventContext.WriteString(job.Params)

	for _, handlerFunc := range job.Handler.HandlerFuncs {
		if err := handlerFunc(&eventContext); err != nil {
			eventContext.SetError(err)
			trace.SetError(err)
		}
	}
}

func (c *cronWorker) getLockKey(handlerName string) string {
	return fmt.Sprintf("%s:cron-worker-lock:%s", c.service.Name(), handlerName)
}
