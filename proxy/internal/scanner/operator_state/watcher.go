package operator_state

import (
	"bufio"
	"context"
	"io"
	"os"
	"runtime"
	"sync"
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
	verifier    *HashChainVerifier
	stopCh      chan struct{}
	wg          sync.WaitGroup
	stopOnce    sync.Once
	initialDone chan struct{} // closed when initial replay completes
	initialErr  error
}

// NewWatcher creates a Watcher for the given config and callbacks.
func NewWatcher(cfg Config, onDecision DecisionCallback, onNote NoteCallback) (*Watcher, error) {
	return &Watcher{
		cfg:         cfg,
		onDecision:  onDecision,
		onNote:      onNote,
		verifier:    NewHashChainVerifier(),
		stopCh:      make(chan struct{}),
		initialDone: make(chan struct{}),
	}, nil
}

// handleDecisionLine parses, hash-verifies, and dispatches one decision line.
// Verify is a no-op under the v1 contract; when v2 ships, tampered entries
// are rejected here without changing any other ingest code.
func (w *Watcher) handleDecisionLine(line []byte, phase string) error {
	ev, err := ParseDecision(line)
	if err != nil {
		log.Warn().Err(err).Str("file", w.cfg.DecisionsPath).Str("phase", phase).Msg("jugeni: skipping malformed decision line")
		return nil // skip, don't abort
	}
	if err := w.verifier.Verify(ev.PrevHash, ev.EntryHash); err != nil {
		log.Warn().Err(err).Str("file", w.cfg.DecisionsPath).Str("decision", ev.Decision).Msg("jugeni: hash chain verification failed; refusing entry")
		return nil
	}
	w.onDecision(ev)
	return nil
}

// handleNoteLine parses, hash-verifies, and dispatches one note line.
func (w *Watcher) handleNoteLine(line []byte, phase string) error {
	ev, err := ParseNote(line)
	if err != nil {
		log.Warn().Err(err).Str("file", w.cfg.NotesPath).Str("phase", phase).Msg("jugeni: skipping malformed note line")
		return nil
	}
	if err := w.verifier.Verify(ev.PrevHash, ev.EntryHash); err != nil {
		log.Warn().Err(err).Str("file", w.cfg.NotesPath).Str("note", ev.Note).Msg("jugeni: hash chain verification failed; refusing entry")
		return nil
	}
	w.onNote(ev)
	return nil
}

// Start begins watching the configured audit log files. It blocks until the
// initial replay of both files is complete, then returns. The tail loop runs
// in background goroutines, resuming from the byte offset the replay consumed.
// Call Stop() to shut down.
func (w *Watcher) Start(ctx context.Context) error {
	var initErr error
	var decisionsOffset, notesOffset int64

	if w.cfg.DecisionsPath != "" {
		offset, err := w.replayFile(w.cfg.DecisionsPath, func(line []byte) error {
			return w.handleDecisionLine(line, "replay")
		})
		if err != nil {
			log.Warn().Err(err).Str("path", w.cfg.DecisionsPath).Msg("jugeni: decision log initial replay failed")
			initErr = err
		}
		decisionsOffset = offset
	}

	if w.cfg.NotesPath != "" {
		offset, err := w.replayFile(w.cfg.NotesPath, func(line []byte) error {
			return w.handleNoteLine(line, "replay")
		})
		if err != nil {
			log.Warn().Err(err).Str("path", w.cfg.NotesPath).Msg("jugeni: note log initial replay failed")
			if initErr == nil {
				initErr = err
			}
		}
		notesOffset = offset
	}

	// Signal that initial replay is done.
	close(w.initialDone)
	w.initialErr = initErr

	// Start tail goroutines from the replay offsets so lines appended between
	// replay and the first poll/notify cycle are never missed.
	if w.cfg.DecisionsPath != "" {
		w.wg.Add(1)
		go w.tailFile(ctx, w.cfg.DecisionsPath, decisionsOffset, func(line []byte) error {
			return w.handleDecisionLine(line, "tail")
		})
	}

	if w.cfg.NotesPath != "" {
		w.wg.Add(1)
		go w.tailFile(ctx, w.cfg.NotesPath, notesOffset, func(line []byte) error {
			return w.handleNoteLine(line, "tail")
		})
	}

	return nil
}

// WaitInitial blocks until the initial file replay is complete.
func (w *Watcher) WaitInitial() error {
	<-w.initialDone
	return w.initialErr
}

// Stop signals the watcher to shut down and waits for the tail goroutines to
// exit. Safe to call more than once.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		close(w.stopCh)
		w.wg.Wait()
	})
}

// replayFile reads lines from path up to the file size at open time and invokes
// the callback for each. It returns the byte offset consumed so the tail loop
// can resume from exactly there — lines appended during or after replay are
// left for the tail to deliver.
func (w *Watcher) replayFile(path string, cb func([]byte) error) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn().Str("path", path).Msg("jugeni: audit log file does not exist yet; will retry on tail")
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := fi.Size()

	scanner := bufio.NewScanner(io.LimitReader(f, size))
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
		return 0, err
	}

	log.Info().Str("path", path).Int("lines", lineNo).Msg("jugeni: initial replay complete")
	return size, nil
}

// tailFile watches path for new lines and invokes cb for each.
// Uses fsnotify on Linux/macOS, polling on Windows.
func (w *Watcher) tailFile(ctx context.Context, path string, offset int64, cb func([]byte) error) {
	defer w.wg.Done()

	usePolling := runtime.GOOS == "windows" || os.Getenv("TAMGA_OPERATOR_STATE_FORCE_POLL") != ""

	if usePolling {
		w.pollTail(ctx, path, offset, cb)
	} else {
		w.fsnotifyTail(ctx, path, offset, cb)
	}
}

// pollTail uses periodic stat() + read to detect new lines, starting from the
// byte offset the initial replay consumed.
func (w *Watcher) pollTail(ctx context.Context, path string, offset int64, cb func([]byte) error) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	lastSize := offset

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

// fsnotifyTail uses inotify/kqueue to detect writes, starting from the byte
// offset the initial replay consumed.
func (w *Watcher) fsnotifyTail(ctx context.Context, path string, offset int64, cb func([]byte) error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("jugeni: fsnotify unavailable; falling back to polling")
		w.pollTail(ctx, path, offset, cb)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(path); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("jugeni: fsnotify Add failed; falling back to polling")
		w.pollTail(ctx, path, offset, cb)
		return
	}

	lastSize := offset

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
