package genembed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"m/orchard/pkg/types/chatmessages"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
	"github.com/aerth/mostly/vector256"
	"uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var model = "nomic-embed-text:v1.5"

var defaultEclient *mllm.E

const MinLength = 3

func BestEffort(ctx context.Context, s string, isQueryOrQuestion bool) *vector256.Vector256 {
	v, err := Embed(ctx, nil, s, isQueryOrQuestion)
	if err != nil {
		slog.Error("best-effort giving up", "err", err)
	}
	if len(v) == 0 {
		return nil
	}
	return &v
}

func GetEmbeddingEndpoint() string {
	if x := os.Getenv("LLM_EMBED_HOST"); x != "" {
		return x
	}
	if x := os.Getenv("MLLM_EMBED_HOST"); x != "" {
		return x
	}
	if x := os.Getenv("OLLAMA_EMBED_HOST"); x != "" {
		return x
	}
	return "http://127.0.0.1:11434"
}

type VEC = vector256.Vector256

var ErrBadQuery = errors.New("bad search query")

func Embed(ctx context.Context, client *mllm.E, s string, isQueryOrQuestion bool) (vector256.Vector256, error) {
	if len(s) < MinLength {
		return nil, stackerr.Errorf("too short: %w", ErrBadQuery)
	}
	if strings.Count(s, " ") > 1024 {
		return nil, fmt.Errorf("chunk too big: %w", ErrBadQuery)
	}
	if client == nil && defaultEclient == nil {
		e := mllm.Default()
		defaultEclient = &e
	}
	if client == nil {
		client = defaultEclient
	}
	var ereq mllm.EmbedRequest
	if isQueryOrQuestion {
		ereq = mllm.NewNomicQuery(s)
	} else {
		ereq = mllm.NewNomicDocument(s)
	}
	got, err := client.Embed(ctx, ereq)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	if len(got.Embeddings) != 1 {
		return nil, stackerr.Errorf("got %d not 1", len(got.Embeddings))
	}
	//slog.Debug("got embedding")
	return vector256.Vector256(got.Embeddings[0]), nil
}

func checkInputQuery(query *vector256.Vector256) error {
	if query == nil {
		return fmt.Errorf("query is nil")
	}
	if len(*query) != 256 {
		return fmt.Errorf("query is not 256: %d", len(*query))
	}
	for _, v := range *query {
		if v != 0 {
			return nil
		}
	}
	return fmt.Errorf("empty query")
}

func QueryDB(ctx context.Context, db *gorm.DB, threadID uuid.UUID, query *vector256.Vector256, limit int) (chatmessages.ChatMessages, error) {
	if err := checkInputQuery(query); err != nil {
		return nil, err
	}
	// The operator <=> calculates cosine distance
	messages, err := gorm.G[chatmessages.ChatMessage](db).
		Where("thread_id = ? AND embedding IS NOT NULL", threadID).
		Order(clause.OrderBy{
			Expression: clause.Expr{
				SQL:  "embedding <=> ?::vector", // Explicit type cast
				Vars: []any{query},
			},
		}).
		Limit(limit).Find(ctx)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

// NomicEmbed ...
// From docs: "Important! The text prompt must include a task instruction prefix, instructing the model which task is being performed."
// "Please use search_query: before your queries/questions, and search_document: before your documents."
func NomicEmbed(text string, isQueryOrQuestion bool) mllm.EmbedRequest {
	if isQueryOrQuestion {
		return mllm.NewNomicQuery(text)
	}
	return mllm.NewNomicDocument(text)
}

var DefaultEmbedOptions map[string]any
