package policy

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// policyReloadDebounce coalesces rapid editor saves into a single reload.
const policyReloadDebounce = 500 * time.Millisecond

// WatchPolicy watches path (typically tamga-policy.yaml) via fsnotify on the parent directory.
// On relevant write/create/rename events, after debouncing it calls store.Reload(path).
// An optional onReload callback fires after each successful policy reload (e.g. for DFA rebuild).
// Parse failures are logged; the previous policy in the store is unchanged.
func WatchPolicy(path string, store *PolicyStore, onReload func()) (stop func(), err error) {
	if store == nil {
		return nil, errors.New("policy store is nil")
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return nil, err
	}

	var mu sync.Mutex
	var timer *time.Timer
	scheduleReload := func() {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(policyReloadDebounce, func() {
			if err := store.Reload(path); err != nil {
				log.Error().Err(err).Str("component", "policy_watcher").Msg("policy reload failed (keeping previous policy)")
				return
			}
			if onReload != nil {
				onReload()
			}
		})
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if !eventTargetsFile(ev.Name, path, base) {
					continue
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove|fsnotify.Chmod) != 0 {
					scheduleReload()
				}
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return func() {
		mu.Lock()
		if timer != nil {
			timer.Stop()
		}
		mu.Unlock()
		_ = w.Close()
		<-done
	}, nil
}

func eventTargetsFile(eventPath, policyPath, base string) bool {
	if eventPath == "" {
		return false
	}
	ep := filepath.Clean(eventPath)
	pp := filepath.Clean(policyPath)
	if strings.EqualFold(ep, pp) {
		return true
	}
	return strings.EqualFold(filepath.Base(ep), base)
}
