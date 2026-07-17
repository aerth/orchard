package embeddings

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"m/pkg/chatmessages"
	"m/pkg/llm"
	"m/pkg/vector256"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var model = "nomic-embed-text:v1.5"

var defaultEclient *llm.Client

const MinLength = 4

func BestEffort(ctx context.Context, s string) *vector256.Vector256 {
	v, err := Embed(ctx, nil, s)
	if err != nil {
		slog.Error("best-effort giving up", "err", err)
	}
	if len(v) == 0 {
		return nil
	}
	return &v
}

func Embed(ctx context.Context, client *llm.Client, s string) (vector256.Vector256, error) {
	if len(s) < MinLength {
		return nil, nil
	}
	if client == nil && defaultEclient == nil {
		defaultEclient = llm.NewClient(&url.URL{Host: "127.0.0.1:11434", Scheme: "http"}, http.DefaultClient)
	}
	if client == nil {
		client = defaultEclient
	}
	ereq := GetEmbeddingRequestQuery(s)
	got, err := client.Embed(ctx, &ereq)
	if err != nil {
		return nil, err
	}
	if len(got.Embeddings) != 1 {
		return nil, fmt.Errorf("got %d not 1", len(got.Embeddings))
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

/*
type OllamaEmbedRequest struct {
	Model   string   `json:"model"`
	Input   []string `json:"input"`
	Options Options  `json:"options"`
}

type Options struct {
	EmbeddingDim int `json:"embedding_dim"` // Truncates vector to 256 for ultra-fast CPU math
}
*/
/*
from config_sentence_transforms.json:

	{
	  "__version__": {
	    "sentence_transformers": "3.3.0",
	    "transformers": "4.44.2",
	    "pytorch": "2.4.1+cu121"
	  },
	  "prompts": {
	    "query": "search_query: ",
	    "passage": "search_document: ",
	    "Classification": "classification: ",
	    "MultilabelClassification": "classification: ",
	    "Clustering": "clustering: ",
	    "PairClassification": "classification: ",
	    "STS": "classification: ",
	    "Summarization": "classification: ",
	    "Speed": "search_document: "
	  },
	  "default_prompt_name": null,
	  "similarity_fn_name": "cosine"
	}
*/

func GetEmbeddingEndpoint() string {
	return os.Getenv("OLLAMA_EMBED_URL")
}

func GetEmbeddingRequestQuery(s string) llm.EmbedRequest {
	reqBody := llm.EmbedRequest{
		Model: model,
		Input: []string{fmt.Sprintf("search_query: %s", s)},
		// Options: map[string]any{
		//		"embedding_dim": 256,
		//	},
		Truncate:   new(true),
		Dimensions: 256,
	}
	return reqBody
}

func GetEmbeddingRequestSearchDocument(s string) llm.EmbedRequest {
	reqBody := llm.EmbedRequest{
		Model:      model,
		Input:      []string{fmt.Sprintf("search_document: %s", s)},
		Truncate:   new(true),
		Dimensions: 256,
		//	Options: map[string]any{
		//		"embedding_dim": 256,
		//	},
	}
	return reqBody
}
