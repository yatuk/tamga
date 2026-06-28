package operator_state

import (
	"bufio"
	"context"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// DecisionCallback is invoked for each parsed decision event during replay and tail.
type DecisionCallback func(DecisionEvent)

// NoteCallback is invoked for each parsed note event during replay and tail.
type NoteCallback func(NoteEvent)

// Watcher tails the jugeni audit log files and invokes callbacks for new entries.
// On startup it replays the entire file to build initial state, then watches for
// appended lines via fsnotify (Linux/macOS) or polling (Windows fallback).
type Watcher struct {
	cfg         Config
	onDecision  DecisionCallback
	onNote      NoteCallback
	stopCh      chan struct{}
	doneCh      chan struct{}
	initialDone chan struct{} // closed when initial replay completes
	initialErr  error
}

// NewWatcher creates a Watcher for the given config and callbacks.
func NewWatcher(cfg Config, onDecision DecisionCallback, onNote NoteCallback) (*Watcher, error) {
	return &Watcher{
		cfg:         cfg,
		onDecision:  onDecision,
		onNote:      onNote,
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		initialDone: make(chan struct{}),
	}, nil
}

// Start begins watching the configured audit log files. It blocks until the
// initial replay of both files is complete, then returns. The tail loop runs
// in background goroutines.
// Call Stop() to shut down.
func (w *Watcher) Start(ctx context.Context) error {
	var initErr error

	if w.cfg.DecisionsPath != "" {
		if err := w.replayFile(w.cfg.DecisionsPath, func(line []byte) error {
			ev, err := ParseDecision(line)
			if err != nil {
				log.Warn().Err(err).Str("file", w.cfg.DecisionsPath).Msg("jugeni: skipping malformed decision line")
				return nil // skip, don't abort replay
			}
			w.onDecision(ev)
			return nil
		}); err != nil {
			log.Warn().Err(err).Str("path", w.cfg.DecisionsPath).Msg("jugeni: decision log initial replay failed")
			initErr = err
		}
	}

	if w.cfg.NotesPath != "" {
		if err := w.replayFile(w.cfg.NotesPath, func(line []byte) error {
			ev, err := ParseNote(line)
			if err != nil {
				log.Warn().Err(err).Str("file", w.cfg.NotesPath).Msg("jugeni: skipping malformed note line")
				return nil
			}
			w.onNote(ev)
			return nil
		}); err != nil {
			log.Warn().Err(err).Str("path", w.cfg.NotesPath).Msg("jugeni: note log initial replay failed")
			if initErr == nil {
				initErr = err
			}
		}
	}

	// Signal that initial replay is done.
	close(w.initialDone)
	w.initialErr = initErr

	// Start tail goroutines.
	if w.cfg.DecisionsPath != "" {
		go w.tailFile(ctx, w.cfg.DecisionsPath, func(line []byte) error {
			ev, err := ParseDecision(line)
			if err != nil {
				log.Warn().Err(err).Str("file", w.cfg.DecisionsPath).Msg("jugeni: skipping malformed decision line in tail")
				return nil
			}
			w.onDecision(ev)
			return nil
		})
	}

	if w.cfg.NotesPath != "" {
		go w.tailFile(ctx, w.cfg.NotesPath, func(line []byte) error {
			ev, err := ParseNote(line)
			if err != nil {
				log.Warn().Err(err).Str("file", w.cfg.NotesPath).Msg("jugeni: skipping malformed note line in tail")
				return nil
			}
			w.onNote(ev)
			return nil
		})
	}

	return nil
}

// WaitInitial blocks until the initial file replay is complete.
func (w *Watcher) WaitInitial() error {
	<-w.initialDone
	return w.initialErr
}

// Stop signals the watcher to shut down and waits for goroutines to exit.
func (w *Watcher) Stop() {
	close(w.stopCh)
	<-w.doneCh
}

// replayFile reads all lines from path and invokes the callback for each.
// It returns the file size at EOF so the tail loop can resume from that offset.
func (w *Watcher) replayFile(path string, cb func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Str("path", path).Msg("jugeni: audit log file does not exist yet; will retry on tail")
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer for large lines (unlikely but safe).
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// Copy the line since scanner reuses its buffer.
		cp := make([]byte, len(line))
		copy(cp, line)
		if err := cb(cp); err != nil {
			log.Warn().Err(err).Int("line", lineNo).Str("path", path).Msg("jugeni: line callback failed")
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	log.Info().Str("path", path).Int("lines", lineNo).Msg("jugeni: initial replay complete")
	return nil
}

// tailFile watches path for new lines and invokes cb for each.
// Uses fsnotify on Linux/macOS, polling on Windows.
func (w *Watcher) tailFile(ctx context.Context, path string, cb func([]byte) error) {
	usePolling := runtime.GOOS == "windows" || os.Getenv("TAMGA_OPERATOR_STATE_FORCE_POLL") != ""

	if usePolling {
		w.pollTail(ctx, path, cb)
	} else {
		w.fsnotifyTail(ctx, path, cb)
	}

	// Signal tail done (one per file; we track via doneCh close on Stop).
}

// pollTail uses periodic stat() + read to detect new lines.
func (w *Watcher) pollTail(ctx context.Context, path string, cb func([]byte) error) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	var lastSize int64

	// Get initial size from initial replay (if file existed).
	if fi, err := os.Stat(path); err == nil {
		lastSize = fi.Size()
	}

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			fi, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					lastSize = 0
					continue
				}
				log.Warn().Err(err).Str("path", path).Msg("jugeni: stat failed during poll")
				continue
			}

			newSize := fi.Size()
			if newSize < lastSize {
				// File was truncated or rotated. Re-read from start.
				log.Warn().Str("path", path).Int64("old_size", lastSize).Int64("new_size", newSize).
					Msg("jugeni: audit log file truncated; re-reading from start")
				lastSize = 0
			}

			if newSize > lastSize {
				if err := w.readRange(path, lastSize, newSize, cb); err != nil {
					log.Warn().Err(err).Str("path", path).Msg("jugeni: readRange failed during poll")
					continue
				}
				lastSize = newSize
			}
		}
	}
}

// fsnotifyTail uses inotify/kqueue to detect writes.
func (w *Watcher) fsnotifyTail(ctx context.Context, path string, cb func([]byte) error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("jugeni: fsnotify unavailable; falling back to polling")
		w.pollTail(ctx, path, cb)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("jugeni: fsnotify Add failed; falling back to polling")
		w.pollTail(ctx, path, cb)
		return
	}

	var lastSize int64
	if fi, err := os.Stat(path); err == nil {
		lastSize = fi.Size()
	}

	for {
		select {
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				fi, err := os.Stat(path)
				if err != nil {
					continue
				}
				newSize := fi.Size()
				if newSize < lastSize {
					lastSize = 0
				}
				if newSize > lastSize {
					if err := w.readRange(path, lastSize, newSize, cb); err != nil {
						log.Warn().Err(err).Str("path", path).Msg("jugeni: readRange failed on fsnotify")
						continue
					}
					lastSize = newSize
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Warn().Err(err).Str("path", path).Msg("jugeni: fsnotify error")
		}
	}
}

// readRange reads bytes [start, end) from path and invokes cb for each complete line.
func (w *Watcher) readRange(path string, start, end int64, cb func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}

	// Read exactly (end - start) bytes.
	limit := end - start
	reader := io.LimitReader(f, limit)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		if err := cb(cp); err != nil {
			log.Warn().Err(err).Str("path", path).Msg("jugeni: tail line callback failed")
		}
	}

	return scanner.Err()
}
