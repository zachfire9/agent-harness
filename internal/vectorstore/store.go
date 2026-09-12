package vectorstore

import (
	"context"
	"fmt"
	"strings"
)

const ProviderNone = "none"

// Config contains vector-store provider settings.
type Config struct {
	Provider string
}

// Document is a provider-neutral representation of content that could be stored for RAG.
type Document struct {
	ID        string
	Content   string
	Metadata  map[string]string
	Embedding []float64
}

// SearchRequest describes a future similarity search without committing to a vendor API.
type SearchRequest struct {
	Query     string
	Embedding []float64
	Limit     int
	Metadata  map[string]string
}

// SearchResult represents one retrieved document and its provider-normalized score.
type SearchResult struct {
	Document Document
	Score    float64
}

// Store isolates vector database operations behind a provider-neutral interface.
type Store interface {
	Provider() string
	Upsert(ctx context.Context, documents []Document) error
	Search(ctx context.Context, request SearchRequest) ([]SearchResult, error)
}

// New selects a vector store implementation from config.
func New(cfg Config) (Store, error) {
	provider := normalizeProvider(cfg.Provider)
	switch provider {
	case "", ProviderNone:
		return NewNoopStore(), nil
	default:
		return nil, fmt.Errorf("unsupported vector store provider %q; supported providers: %s", provider, ProviderNone)
	}
}

// NoopStore satisfies Store without connecting to a real vector database.
type NoopStore struct{}

// NewNoopStore creates a disabled vector store for early app iterations.
func NewNoopStore() NoopStore {
	return NoopStore{}
}

// Provider returns the configured provider name.
func (NoopStore) Provider() string { return ProviderNone }

// Upsert accepts documents but intentionally does not persist them.
func (NoopStore) Upsert(ctx context.Context, documents []Document) error {
	return ctx.Err()
}

// Search returns no matches without network or database access.
func (NoopStore) Search(ctx context.Context, request SearchRequest) ([]SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []SearchResult{}, nil
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}
