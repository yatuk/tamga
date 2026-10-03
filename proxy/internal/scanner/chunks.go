package scanner

import (
	"context"
	"runtime"
	"sync"
	"unicode/utf8"

	"github.com/yatuk/tamga/internal/extract"
)

// The scanners run dozens of regular expressions over the text, one after
// another, on one core. That is fine for a chat message and far too slow for
// what an agent sends: a megabyte of file contents took four seconds. A long
// segment is therefore cut into chunks that are scanned in parallel.
const (
	// chunkSize is the most text one scan handles. Short enough that a
	// megabyte spreads over every core, long enough that a chat message is
	// never cut.
	chunkSize = 32 << 10
	// chunkOverlap is how much neighbouring chunks share, so that something
	// lying across a cut is whole in one of them. It bounds the longest
	// match that can be found across a cut.
	chunkOverlap = 2 << 10
	// cutWindow is how far back from the size limit a cut may move to land
	// on whitespace rather than inside a word or a number.
	cutWindow = 1 << 10
)

// chunk is a piece of a segment's text: text[start:end].
type chunk struct{ start, end int }

// splitChunks cuts text into overlapping chunks of at most chunkSize bytes.
// Cuts fall on whitespace when there is any nearby, and never inside a
// UTF-8 sequence.
func splitChunks(text string) []chunk {
	if len(text) <= chunkSize {
		return []chunk{{0, len(text)}}
	}
	var out []chunk
	start := 0
	for {
		end := start + chunkSize
		if end >= len(text) {
			out = append(out, chunk{start, len(text)})
			return out
		}
		end = cutPoint(text, end)
		out = append(out, chunk{start, end})
		next := cutPoint(text, end-chunkOverlap)
		if next <= start {
			// No usable cut in the overlap (one enormous token): move on
			// without one rather than loop.
			next = end
		}
		start = next
	}
}

// cutPoint returns a position at or before limit that is safe to cut at:
// just after whitespace if there is some within cutWindow, otherwise the
// nearest rune boundary.
func cutPoint(text string, limit int) int {
	if limit >= len(text) {
		return len(text)
	}
	floor := limit - cutWindow
	if floor < 0 {
		floor = 0
	}
	for i := limit; i > floor; i-- {
		switch text[i-1] {
		case ' ', '\n', '\t', '\r':
			return i
		}
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return limit
}

// scanUnit is one chunk of one segment, and what was found in it.
type scanUnit struct {
	seg   int
	chunk chunk
	found []Finding
	err   error
}

// scanUnits scans every chunk of every segment and returns the findings per
// segment, positions relative to the segment's text.
//
// Chunks run in parallel unless the pipeline uses the worker pool: that pool
// is already the concurrency limit there, and filling its queue from here
// would make it shed scanners.
func (r *Registry) scanUnits(ctx context.Context, segs []extract.Segment, cfg PipelineConfig) ([][]Finding, error) {
	var units []*scanUnit
	for i, seg := range segs {
		for _, c := range splitChunks(seg.Text) {
			units = append(units, &scanUnit{seg: i, chunk: c})
		}
	}

	run := func(u *scanUnit) {
		seg := segs[u.seg]
		part := seg
		part.Text = seg.Text[u.chunk.start:u.chunk.end]
		found, err := r.ScanAllWithConfig(ctx, []byte(part.Text), cfg)
		for j := range found {
			placeFinding(&found[j], u.seg, part)
		}
		found = r.dropEchoes(ctx, found, u.seg, part, cfg)
		// Positions were relative to the chunk; make them relative to the
		// segment.
		for j := range found {
			if found[j].Placed() {
				found[j].StartPos += u.chunk.start
				found[j].EndPos += u.chunk.start
			}
		}
		u.found, u.err = found, err
	}

	workers := runtime.GOMAXPROCS(0)
	if cfg.Pool != nil || cfg.Mode == ModeWorkerPool || len(units) == 1 {
		workers = 1
	}
	if workers > len(units) {
		workers = len(units)
	}
	if workers <= 1 {
		for _, u := range units {
			run(u)
		}
	} else {
		var wg sync.WaitGroup
		next := make(chan *scanUnit)
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for u := range next {
					run(u)
				}
			}()
		}
		for _, u := range units {
			next <- u
		}
		close(next)
		wg.Wait()
	}

	perSeg := make([][]Finding, len(segs))
	var firstErr error
	for _, u := range units {
		if u.err != nil && firstErr == nil {
			firstErr = u.err
		}
		perSeg[u.seg] = append(perSeg[u.seg], u.found...)
	}
	for i := range perSeg {
		perSeg[i] = dropOverlapDuplicates(perSeg[i])
	}
	return perSeg, firstErr
}

// dropOverlapDuplicates removes what two neighbouring chunks both found in
// the text they share.
func dropOverlapDuplicates(in []Finding) []Finding {
	if len(in) < 2 {
		return in
	}
	type key struct {
		typ, cat, match string
		start, end      int
	}
	seen := make(map[key]bool, len(in))
	out := in[:0:0]
	for _, f := range in {
		k := key{typ: f.Type, cat: f.Category, start: f.StartPos, end: f.EndPos}
		if !f.Placed() {
			k.match = f.Match
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}
