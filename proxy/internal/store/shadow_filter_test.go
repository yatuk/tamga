package store

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yatuk/tamga/internal/events"
	"github.com/yatuk/tamga/internal/providers"
)

// shadowProviders are recorded providers of every kind the filter has to
// tell apart: unknown ones, the known ones in any case and with spaces
// around them, the LiteLLM bridge, and no provider at all.
var shadowProviders = []string{
	"deepseek", "mistral", "DeepSeek", " groq ", "openai2", "azure-openai", "litellm_proxy",
	"openai", "OpenAI", " anthropic ", "google", "azure", "azure_openai", "google_vertex",
	"litellm", "LiteLLM",
	"", "  ",
}

// The database's filter is given the one list as an argument: no provider
// name is written into the statement, here or anywhere else in the search.
func TestShadowConditionTakesTheListAsAnArgument(t *testing.T) {
	cond, known := shadowCondition(4)
	if !reflect.DeepEqual(known, providers.NotShadow()) {
		t.Errorf("the filter is given %v, want providers.NotShadow() %v", known, providers.NotShadow())
	}
	if !strings.Contains(cond, "$4::text[]") {
		t.Errorf("condition does not take argument 4: %s", cond)
	}

	for _, p := range []EventSearchParams{{ShadowOnly: true}, {Provider: "shadow"}, {Provider: " Shadow "}, {ShadowOnly: true, Provider: "openai"}} {
		where, args := eventSearchWhere(uuid.MustParse(testOrgID), p)
		for _, name := range providers.NotShadow() {
			if strings.Contains(where, name) {
				t.Errorf("%+v: the statement names %q: %s", p, name, where)
			}
		}
		if strings.Contains(where, "NOT IN") {
			t.Errorf("%+v: a second list in the statement: %s", p, where)
		}
		// The list is the argument the condition points at.
		found := false
		for i, a := range args {
			if list, ok := a.([]string); ok && reflect.DeepEqual(list, providers.NotShadow()) {
				found = strings.Contains(where, fmt.Sprintf("ANY($%d::text[])", i+1))
			}
		}
		if !found {
			t.Errorf("%+v: the list is not the argument the condition uses: %s %v", p, where, args)
		}
	}

	// Without the filter there is no such argument.
	where, args := eventSearchWhere(uuid.MustParse(testOrgID), EventSearchParams{Provider: "openai"})
	if strings.Contains(where, "text[]") || len(args) != 4 {
		t.Errorf("a provider filter: %s %v", where, args)
	}
}

// TestPG_ShadowFilterAgreesWithMemory stores one request for each provider
// and asks both sides which are shadow AI: the search of the table and the
// filter over the events a process holds in memory. Needs TAMGA_TEST_DB_URL.
func TestPG_ShadowFilterAgreesWithMemory(t *testing.T) {
	skipIfNoDB(t)
	s := newTestPostgresStore(t)
	defer func() { _ = s.Close() }()
	pgCleanTruncate(t, s)
	ctx := context.Background()
	at := time.Now().UTC().Add(-time.Hour)

	var fromMemory, isShadow []string
	for i, p := range shadowProviders {
		id := fmt.Sprintf("shadow-%02d", i)
		pgInsertAt(t, s, testOrgID, id, p, "some-model", "pass", at, 1, `[]`, 0)
		e := events.Event{RequestID: id, Provider: p, Timestamp: at}
		if events.MatchEventSearch(e, "", "", true, "", "", "", "", "", time.Time{}, time.Time{}) {
			fromMemory = append(fromMemory, id)
		}
		if providers.IsShadow(p) {
			isShadow = append(isShadow, id)
		}
	}

	for _, p := range []EventSearchParams{{ShadowOnly: true, Limit: 100}, {Provider: "shadow", Limit: 100}} {
		rows, total, err := s.SearchSecurityEvents(ctx, testOrgID, p)
		if err != nil {
			t.Fatalf("SearchSecurityEvents(%+v): %v", p, err)
		}
		var fromTable []string
		for _, r := range rows {
			fromTable = append(fromTable, r.RequestID)
		}
		sort.Strings(fromTable)
		if !reflect.DeepEqual(fromTable, fromMemory) || total != len(fromMemory) {
			t.Errorf("%+v:\n the table says %v (total %d)\n memory says   %v", p, fromTable, total, fromMemory)
		}
	}
	if !reflect.DeepEqual(fromMemory, isShadow) {
		t.Errorf("memory says %v, providers.IsShadow says %v", fromMemory, isShadow)
	}
	if len(fromMemory) != 7 {
		t.Errorf("%d of the providers are shadow, want the 7 unknown ones: %v", len(fromMemory), fromMemory)
	}
}
