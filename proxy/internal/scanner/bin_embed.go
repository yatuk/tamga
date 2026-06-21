package scanner

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/yatuk/tamga/internal/scanner/bindata"
)

// InitBINLookupEmbed loads the BIN database from the embedded CSV.
// This is the production path — the CSV is compiled into the binary via //go:embed.
// Falls back to a warning log if the embedded data is empty (should never happen).
func InitBINLookupEmbed() error {
	if len(bindata.BinlistCSV) == 0 {
		log.Warn().Str("component", "bin_lookup").Msg("embedded BIN data is empty — BIN lookups will be unavailable")
		return fmt.Errorf("embedded BIN data is empty")
	}

	r := csv.NewReader(bytes.NewReader(bindata.BinlistCSV))
	// Skip header
	if _, err := r.Read(); err != nil && err != io.EOF {
		return fmt.Errorf("read bin header: %w", err)
	}

	lookup := &BINLookup{trie: newBINTrie()}
	count := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) < 7 {
			continue
		}
		bin := strings.TrimSpace(rec[0])
		if len(bin) < 6 || len(bin) > 8 {
			continue
		}
		entry := &BINEntry{
			BIN:         bin,
			Brand:       strings.TrimSpace(rec[1]),
			Type:        strings.TrimSpace(rec[2]),
			Category:    strings.TrimSpace(rec[3]),
			CountryCode: strings.TrimSpace(rec[5]),
			BankName:    strings.TrimSpace(rec[6]),
		}
		lookup.trie.Insert(bin, entry)
		count++
	}

	globalBINLookup = lookup
	log.Debug().Str("component", "bin_lookup").Int("count", count).Msg("BIN database loaded from embedded CSV")
	return nil
}
