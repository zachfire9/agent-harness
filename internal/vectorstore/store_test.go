package vectorstore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zachfire9/agent-harness/internal/vectorstore"
)

func TestNewDefaultStoreSelectsNoopProvider(t *testing.T) {
	store, err := vectorstore.New(vectorstore.Config{Provider: ""})
	if err != nil {
		t.Fatalf("expected default vector store, got error: %v", err)
	}
	if store.Provider() != "none" {
		t.Fatalf("expected none provider, got %q", store.Provider())
	}
}

func TestNoopStoreImplementsDocumentUpsertAndSimilaritySearch(t *testing.T) {
	var store vectorstore.Store = vectorstore.NewNoopStore()

	err := store.Upsert(context.Background(), []vectorstore.Document{{
		ID:      "doc-1",
		Content: "hello rag",
		Metadata: map[string]string{
			"source": "test",
		},
		Embedding: []float64{0.1, 0.2},
	}})
	if err != nil {
		t.Fatalf("expected noop upsert to succeed, got %v", err)
	}

	matches, err := store.Search(context.Background(), vectorstore.SearchRequest{
		Query:     "rag",
		Embedding: []float64{0.1, 0.2},
		Limit:     5,
	})
	if err != nil {
		t.Fatalf("expected noop search to succeed, got %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected noop search to return no matches, got %#v", matches)
	}
}

func TestNewRejectsUnsupportedProvider(t *testing.T) {
	_, err := vectorstore.New(vectorstore.Config{Provider: "pinecone"})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "unsupported vector store provider") {
		t.Fatalf("expected controlled unsupported provider error, got %q", err.Error())
	}
}

func TestNewAcceptsExplicitNoneProvider(t *testing.T) {
	store, err := vectorstore.New(vectorstore.Config{Provider: " none "})
	if err != nil {
		t.Fatalf("expected explicit none provider, got error: %v", err)
	}
	if store.Provider() != "none" {
		t.Fatalf("expected none provider, got %q", store.Provider())
	}
}
