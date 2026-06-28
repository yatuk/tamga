package scanner

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

type BINEntry struct {
	BIN         string
	Brand       string
	Type        string
	Category    string
	CountryCode string
	BankName    string
}

type binTrieNode struct {
	children [10]*binTrieNode
	entry    *BINEntry
}

type binTrie struct {
	root *binTrieNode
}

func newBINTrie() *binTrie {
	return &binTrie{root: &binTrieNode{}}
}

func (t *binTrie) Insert(bin string, entry *BINEntry) {
	if t == nil || t.root == nil || entry == nil {
		return
	}
	node := t.root
	for i := 0; i < len(bin); i++ {
		d := bin[i] - '0'
		if d > 9 {
			return
		}
		if node.children[d] == nil {
			node.children[d] = &binTrieNode{}
		}
		node = node.children[d]
	}
	node.entry = entry
}

func (t *binTrie) Lookup(digits string) *BINEntry {
	if t == nil || t.root == nil {
		return nil
	}
	node := t.root
	var last *BINEntry
	for i := 0; i < len(digits); i++ {
		d := digits[i] - '0'
		if d > 9 || node.children[d] == nil {
			break
		}
		node = node.children[d]
		if node.entry != nil {
			last = node.entry
		}
	}
	return last
}

type BINLookup struct {
	trie *binTrie
}

var globalBINLookup *BINLookup

// InitBINLookup loads the BIN lookup table from a CSV file path.
func InitBINLookup(csvPath string) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open bin csv: %w", err)
	}
	defer func() { _ = file.Close() }()

	r := csv.NewReader(file)
	// Best-effort header skip
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
	log.Debug().Str("component", "bin_lookup").Int("count", count).Msg("BIN database loaded")
	return nil
}

// LookupBIN returns the issuer name for a BIN prefix.
func LookupBIN(cardNumber string) *BINEntry {
	if globalBINLookup == nil || globalBINLookup.trie == nil {
		return nil
	}
	digits := digitsOnly(cardNumber)
	if len(digits) < 6 {
		return nil
	}
	if len(digits) >= 8 {
		if entry := globalBINLookup.trie.Lookup(digits[:8]); entry != nil {
			return entry
		}
	}
	return globalBINLookup.trie.Lookup(digits[:6])
}
