package notes

import (
	"fmt"
	"testing"
	"time"
)

// generateNotes creates n creation notes with events and comments for benchmarking.
func generateNotes(n int) (*Index, int) {
	idx := NewIndex()
	totalNotes := 0

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		id := fmt.Sprintf("tic-%04d", i)
		kind := "ticket"
		if i%5 == 0 {
			kind = "warning"
		}
		if i%7 == 0 {
			kind = "review"
		}

		tags := []string{fmt.Sprintf("tag-%d", i%10)}
		if i%3 == 0 {
			tags = append(tags, "critical")
		}

		target := fmt.Sprintf("src/file-%d.ts", i%100)

		creation := &Note{
			ID:   id,
			Kind: kind,
			Type: "task",
			Tags: tags,
			Edges: []Edge{
				{Type: "targets", Target: EdgeTarget{Kind: "path", Ref: target}},
			},
			Body: fmt.Sprintf("# Ticket %d\n\nDescription for ticket %d.", i, i),
			Time: baseTime.Add(time.Duration(i) * time.Hour),
		}
		idx.Ingest(creation)
		totalNotes++

		// Add 2 events per note
		for j := 0; j < 2; j++ {
			ev := &Note{
				Kind: "event",
				Edges: []Edge{
					{Type: "updates", Target: EdgeTarget{Kind: "note", Ref: id}},
				},
				Field: "status",
				Value: "in_progress",
				Time:  baseTime.Add(time.Duration(i)*time.Hour + time.Duration(j+1)*time.Minute),
			}
			idx.Ingest(ev)
			totalNotes++
		}

		// Add 1 comment per note
		comment := &Note{
			Kind: "comment",
			Edges: []Edge{
				{Type: "on", Target: EdgeTarget{Kind: "note", Ref: id}},
			},
			Body: fmt.Sprintf("Progress update for %s", id),
			Time: baseTime.Add(time.Duration(i)*time.Hour + 5*time.Minute),
		}
		idx.Ingest(comment)
		totalNotes++
	}

	return idx, totalNotes
}

func BenchmarkIndex_Build_10k(b *testing.B) {
	idx, _ := generateNotes(10000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Build()
	}
}

func BenchmarkIndex_Query_Kind(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Query(FindOptions{Kind: "ticket"})
	}
}

func BenchmarkIndex_Query_Status(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Query(FindOptions{Status: "in_progress"})
	}
}

func BenchmarkIndex_Query_Target(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Query(FindOptions{Target: "src/file-42.ts"})
	}
}

func BenchmarkIndex_Context(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.ContextForPath("src/file-42.ts")
	}
}

func BenchmarkIndex_Fold_Single(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		creation := idx.CreationNotes["tic-0042"]
		events := idx.EventsByTarget["tic-0042"]
		FoldEvents(creation, events)
	}
}

func BenchmarkIndex_List_All(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.QueryList(ListOptions{SortBy: "created"})
	}
}

func BenchmarkIndex_ResolveID_Exact(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.ResolveID("tic-5000")
	}
}

func BenchmarkIndex_ResolveID_Partial(b *testing.B) {
	idx, _ := generateNotes(10000)
	idx.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.ResolveID("5000")
	}
}

// TestBenchmarkBaseline runs the 10k benchmark with timing assertions.
func TestBenchmarkBaseline(t *testing.T) {
	idx, totalNotes := generateNotes(10000)
	t.Logf("Generated %d total notes (10000 creation + events + comments)", totalNotes)

	// Build
	start := time.Now()
	idx.Build()
	buildTime := time.Since(start)
	t.Logf("Build: %v", buildTime)
	if buildTime > time.Second {
		t.Errorf("Build took %v, target is <1s", buildTime)
	}

	// Verify
	if len(idx.States) != 10000 {
		t.Errorf("States = %d, want 10000", len(idx.States))
	}

	// Query
	start = time.Now()
	results := idx.Query(FindOptions{Kind: "ticket"})
	queryTime := time.Since(start)
	t.Logf("Query(kind=ticket): %v, %d results", queryTime, len(results))
	if queryTime > 200*time.Millisecond {
		t.Errorf("Query took %v, target is <200ms", queryTime)
	}

	// List
	start = time.Now()
	summaries := idx.QueryList(ListOptions{SortBy: "created"})
	listTime := time.Since(start)
	t.Logf("List(all, sorted): %v, %d results", listTime, len(summaries))
	if listTime > 200*time.Millisecond {
		t.Errorf("List took %v, target is <200ms", listTime)
	}

	// Fold single
	start = time.Now()
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("tic-%04d", i)
		creation := idx.CreationNotes[id]
		events := idx.EventsByTarget[id]
		FoldEvents(creation, events)
	}
	foldTime := time.Since(start)
	t.Logf("Fold 1000 notes: %v (avg %v per fold)", foldTime, foldTime/1000)
	if foldTime/1000 > time.Millisecond {
		t.Errorf("Fold avg %v, target is <1ms", foldTime/1000)
	}

	// Context
	start = time.Now()
	ctx := idx.ContextForPath("src/file-42.ts")
	contextTime := time.Since(start)
	t.Logf("Context(src/file-42.ts): %v, %d notes", contextTime, len(ctx))

	// ResolveID
	start = time.Now()
	for i := 0; i < 10000; i++ {
		idx.ResolveID("tic-5000")
	}
	resolveTime := time.Since(start)
	t.Logf("ResolveID x10000: %v (avg %v)", resolveTime, resolveTime/10000)
}


// BenchmarkSearch_Corpus measures end-to-end Search latency across a synthetic
// 1000-note corpus. Used to regress-protect the persisted text-index fast path.
func BenchmarkSearch_Corpus(b *testing.B) {
	idx := NewIndex()
	for i := 0; i < 1000; i++ {
		idx.Ingest(&Note{
			ID:        fmt.Sprintf("t-%04d", i),
			Kind:      "ticket",
			Title:     fmt.Sprintf("ticket %d about auth and rate limiting", i),
			Body:      fmt.Sprintf("body %d describing the issue in detail", i),
			Tags:      []string{"auth", "perf"},
			Timestamp: "2026-04-10T10:00:00Z",
			TargetOID: fmt.Sprintf("oid-%04d", i),
		})
	}
	idx.Build()
	ti := &TextIndex{}
	ti.build(idx.States)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ti.Search("auth rate", 10)
	}
}
