package localfs

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultFlushMaxPending = 256 << 20
	flushMaxBatch          = 4096
)

var errFlusherClosed = errors.New("localfs flusher is closed")

type flushJob struct {
	file       *os.File
	dirRel     string
	path       string
	size       int64
	done       chan error
	superseded atomic.Bool
}

type flusher struct {
	mode       SyncMode
	workers    int
	jobs       chan *flushJob
	maxDelay   time.Duration
	openDir    func(dirRel string) (*os.File, error)
	mu         sync.Mutex
	cond       *sync.Cond
	pending    int64
	maxPending int64
	closed     bool
	finished   chan struct{}
	sendMu     sync.RWMutex
	latestMu   sync.Mutex
	latest     map[string]*flushJob
}

func newFlusher(mode SyncMode, workers int, maxPending int64, maxDelay time.Duration, openDir func(dirRel string) (*os.File, error)) *flusher {
	if 1 > workers {
		workers = runtime.NumCPU()
	}
	if 0 >= maxPending {
		maxPending = defaultFlushMaxPending
	}
	f := &flusher{
		mode:       mode,
		workers:    workers,
		jobs:       make(chan *flushJob, flushMaxBatch),
		maxDelay:   max(maxDelay, 0),
		openDir:    openDir,
		maxPending: maxPending,
		finished:   make(chan struct{}),
		latest:     map[string]*flushJob{},
	}
	f.cond = sync.NewCond(&f.mu)
	go f.run()
	return f
}

func (f *flusher) submit(job *flushJob) error {
	f.mu.Lock()
	for !f.closed && 0 < f.pending && f.maxPending < f.pending+job.size {
		f.cond.Wait()
	}
	if f.closed {
		f.mu.Unlock()
		return errFlusherClosed
	}
	f.pending += job.size
	f.mu.Unlock()
	f.supersede(job)
	f.sendMu.RLock()
	defer f.sendMu.RUnlock()
	if f.isClosed() {
		f.release(job.size)
		return errFlusherClosed
	}
	f.jobs <- job
	return nil
}

func (f *flusher) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *flusher) release(size int64) {
	f.mu.Lock()
	f.pending -= size
	f.cond.Broadcast()
	f.mu.Unlock()
}

func (f *flusher) supersede(job *flushJob) {
	if "" == job.path {
		return
	}
	f.latestMu.Lock()
	if previous, ok := f.latest[job.path]; ok && nil == previous.done {
		previous.superseded.Store(true)
	}
	if nil == job.done && nil != job.file {
		f.latest[job.path] = job
	} else {
		delete(f.latest, job.path)
	}
	f.latestMu.Unlock()
}

func (f *flusher) forget(path string) {
	f.latestMu.Lock()
	if previous, ok := f.latest[path]; ok {
		previous.superseded.Store(true)
		delete(f.latest, path)
	}
	f.latestMu.Unlock()
}

func (f *flusher) settled(job *flushJob) {
	if "" == job.path {
		return
	}
	f.latestMu.Lock()
	if current, ok := f.latest[job.path]; ok && current == job {
		delete(f.latest, job.path)
	}
	f.latestMu.Unlock()
}

func (f *flusher) run() {
	defer close(f.finished)
	for job := range f.jobs {
		batch := []*flushJob{job}
		if 0 < f.maxDelay {
			batch = f.collectFor(batch)
		}
		f.flush(f.drain(batch))
	}
}

func (f *flusher) collectFor(batch []*flushJob) []*flushJob {
	timer := time.NewTimer(f.maxDelay)
	defer timer.Stop()
	for flushMaxBatch > len(batch) {
		select {
		case next, ok := <-f.jobs:
			if !ok {
				return batch
			}
			batch = append(batch, next)
		case <-timer.C:
			return batch
		}
	}
	return batch
}

func (f *flusher) drain(batch []*flushJob) []*flushJob {
	for flushMaxBatch > len(batch) {
		select {
		case next, ok := <-f.jobs:
			if !ok {
				return batch
			}
			batch = append(batch, next)
		default:
			return batch
		}
	}
	return batch
}

func (f *flusher) flush(batch []*flushJob) {
	handles := make([]*os.File, 0, len(batch))
	fileIndex := make([]int, len(batch))
	for i, job := range batch {
		fileIndex[i] = -1
		if nil != job.file && !job.superseded.Load() {
			fileIndex[i] = len(handles)
			handles = append(handles, job.file)
		}
	}
	dirIndex := make(map[string]int)
	dirOpenErr := make(map[string]error)
	dirHandles := make([]*os.File, 0)
	for _, job := range batch {
		if "" == job.dirRel {
			continue
		}
		if _, seen := dirIndex[job.dirRel]; seen {
			continue
		}
		if _, failed := dirOpenErr[job.dirRel]; failed {
			continue
		}
		dir, err := f.openDir(job.dirRel)
		if nil != err {
			dirOpenErr[job.dirRel] = err
			continue
		}
		dirIndex[job.dirRel] = len(handles)
		handles = append(handles, dir)
		dirHandles = append(dirHandles, dir)
	}
	errs := syncFiles(handles, f.mode, f.workers)
	for _, dir := range dirHandles {
		_ = dir.Close()
	}
	for i, job := range batch {
		var err error
		if nil != job.file {
			if 0 <= fileIndex[i] {
				err = errs[fileIndex[i]]
			}
			if closeErr := job.file.Close(); nil == err {
				err = closeErr
			}
			f.settled(job)
		}
		if "" != job.dirRel && nil == err {
			if openErr, failed := dirOpenErr[job.dirRel]; failed {
				err = openErr
			} else {
				err = errs[dirIndex[job.dirRel]]
			}
		}
		f.release(job.size)
		if nil != job.done {
			job.done <- err
		}
	}
}

func (f *flusher) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.cond.Broadcast()
	f.mu.Unlock()
	f.sendMu.Lock()
	close(f.jobs)
	f.sendMu.Unlock()
	<-f.finished
}

func parallelSync(files []*os.File, workers int, fn func(*os.File) error) []error {
	errs := make([]error, len(files))
	if 0 == len(files) {
		return errs
	}
	workers = max(1, min(workers, len(files)))
	var next atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if len(files) <= i {
					return
				}
				errs[i] = fn(files[i])
			}
		}()
	}
	wg.Wait()
	return errs
}
