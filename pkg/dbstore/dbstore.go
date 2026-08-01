// Package dbstore for storing/loading message threads from DB
package dbstore

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log"
	"log/slog"
	"slices"

	"m/pkg/chatmessages"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrClosed = errors.New("closed")

type (
	PostgresDBStore = Store
	Underlying      interface {
		DB() *gorm.DB
	}
)

func New(db Underlying) *Store {
	return &Store{
		db: db,
	}
}
func (*Store) IsDB() {}

func (s *Store) DB() *gorm.DB {
	if s == nil {
		panic("nil store")
	}
	if s.db == nil {
		return nil
	}
	return s.db.DB()
}

type Store struct {
	db Underlying
}

func (s *Store) Add(ctx context.Context, threadID uuid.UUID, msgsInput ...*chatmessages.ChatMessage) error {
	if len(msgsInput) == 0 {
		slog.Warn("no messages given to add")
		return nil
	}
	//slog.Debug("adding to db...", "threadID", threadID, "msgs", len(msgs))

	msgs := slices.DeleteFunc(msgsInput, func(v *chatmessages.ChatMessage) bool {
		if v == nil {
			return true
		}
		if v.DBID != 0 {
			return true
		}
		if v.Ephemeral {
			return true
		}
		if v.ThreadID == uuid.Nil {
			slog.Warn("Add skipping no threadID", "dbid", v.DBID, "is_ephemeral", v.Ephemeral, "'s", v.String(), "content", v.Content, "already", already.Content)
			return true
		}
		return false
	})
	if len(msgs) == 0 {
		slog.Warn("no valid storable messages", "in", len(msgsInput))
		return nil
	}
	for i := range msgs {
		msgs[i].ThreadID = threadID
		if msgs[i].MessageID == (uuid.UUID{}) {
			msgs[i].MessageID = uuid.New()
		}
	}
	m := msgs
	err := gorm.G[*chatmessages.ChatMessage](s.db.DB()).CreateInBatches(ctx, &m, 25)
	if err != nil {
		return fmt.Errorf("create rows: %w", err)
	}
	slog.Debug("Added message to thread DB", "threadID", threadID, "n", len(msgs), "first", msgs[0].String())

	return nil
}

func (s *Store) GetMessage(ctx context.Context, threadID uuid.UUID, messageID uuid.UUID) (chatmessages.ChatMessage, error) {
	return gorm.G[chatmessages.ChatMessage](s.db.DB()).
		Where("role is NOT NULL and message_id = ? and thread_id = ?", messageID, threadID).Order("id desc").First(ctx)
}

//.Omit("embedding")

func (s *Store) SaveThread(ctx context.Context, threadID uuid.UUID, msgs chatmessages.ChatMessages) error {
	if threadID == uuid.Nil {
		slog.Debug("not saving thread to db...", "threadID", threadID, "msgs", len(msgs), "msg1", msgs[len(msgs)-1])
		return nil
	}
	slog.Debug("saving thread to db...", "threadID", threadID, "msgs", len(msgs), "msg1", msgs[len(msgs)-1])

	for i := range msgs {
		msgs[i].ThreadID = threadID
		if msgs[i].MessageID == (uuid.UUID{}) {
			msgs[i].MessageID = uuid.New()
		}
		//	log.Printf("%#v", msgs[i])
	}
	msgs2 := slices.Clone(msgs)
	msgs = slices.DeleteFunc(msgs2, func(cm chatmessages.ChatMessage) bool {
		return cm.DBID != 0 || cm.IsEmpty() || cm.Ephemeral
	})
	var m []chatmessages.ChatMessage = msgs // NOTE: not ChatMessages here for db batches
	err := gorm.G[chatmessages.ChatMessage](s.db.DB()).CreateInBatches(ctx, &m, 25)
	if err != nil {
		return fmt.Errorf("create rows: %w", err)
	}
	slog.Info("Saved thread to DB", "threadID", threadID, "new_msgs", len(msgs), "roles", msgs.FormatRoles())

	return nil
}

func (s *Store) LoadThread(ctx context.Context, threadID uuid.UUID, limit int) (chatmessages.ChatMessages, error) {
	msgs, err := gorm.G[chatmessages.ChatMessage](s.db.DB()).
		//.Omit("embedding")
		Where("role is NOT NULL and thread_id = ?", threadID).Order("id desc").Limit(limit).Find(ctx)
	if err != nil {
		return nil, err
	}
	slices.Reverse(msgs) // reverse dbid desc for limit back to chronological
	slog.Info("loaded messages from db", "threadID", threadID, "n", len(msgs), "roles", chatmessages.ChatMessages(msgs).FormatRoles())
	for i := range msgs {
		//	if msgs[i].Role == "" {
		//		err = stackerr.Errorf("uhoh: %s", string(ToJSON(msgs[i])))
		//	}

		log.Printf("recalled message: %d: %s", i, msgs[i])
		msgs[i].ThreadID = threadID
	}
	return msgs, err
}

func (s *Store) DeleteThread(ctx context.Context, threadID uuid.UUID) error {
	/*
		db, err := s.db.DB().DB()
		if err != nil {
			return fmt.Errorf("no db in db: %w", err)
		}*/

	n, err := gorm.G[chatmessages.ChatMessage](s.db.DB()).Where("thread_id = ?", threadID).Delete(ctx)
	if err != nil {
		return err
	}
	slog.Info("deleted thread", "n", n, "thread", threadID)
	//_, err = db.ExecContext(ctx, `DELETE FROM chat_messages WHERE thread_id=$1`, threadID)
	if err != nil {
		return fmt.Errorf("delete thread %s: %w", threadID, err)
	}
	return nil
}

// FindAll is a generic function that returns an iterator for any model T.
// It accepts a func(*gorm.DB) *gorm.DB to allow the caller to add
// Where, Order, or Joins clauses before iteration begins.
func FindAll[T any](db *gorm.DB, queryOptions ...func(*gorm.DB) *gorm.DB) iter.Seq[T] {
	return func(yield func(T) bool) {
		var results []T

		// 1. Start with the base DB object
		tx := db

		// 2. Apply any custom filters/ordering passed by the caller
		for _, option := range queryOptions {
			tx = option(tx)
		}

		// 3. Execute the query
		err := tx.Find(&results).Error
		if err != nil {
			fmt.Printf("error fetching records: %v\n", err)
			return // Stop iteration if error occurs
		}

		// 4. Yield each item to the caller
		for _, item := range results {
			// If yield returns false, the caller used 'break', so we stop
			if !yield(item) {
				return
			}
		}
	}
}

type Identifiable interface {
	GetID() uint
}
type IterOptions struct {
	Reverse     bool
	PageSize    int
	Filter      any
	FilterQuery []any
}

// GormIter to range over. Or
//
// A **Generics-based, Keyset-paginated, Direction-aware iterator** that solves the "Slow Reader/Drift" problem without the overhead of channels or the complexity of manual `Stop()` calls.
func GormIter[T Identifiable](db *gorm.DB, o *IterOptions) iter.Seq[T] {
	sortBy := "id ASC"
	cmp := ">"
	if o != nil && o.Reverse {
		sortBy = "id DESC"
		cmp = "<"
	}
	pageSize := 10
	if o != nil && o.PageSize != 0 {
		pageSize = o.PageSize
	}
	return func(yield func(T) bool) {
		var lastID uint
		for {
			var batch []T
			query := db.Limit(pageSize).Order(sortBy)

			if o != nil && o.Filter != nil {
				query = query.Where(o.Filter, o.FilterQuery...)
			}

			if lastID != 0 {
				query = query.Where("id "+cmp+" ?", lastID)
			}
			if err := query.Find(&batch).Error; err != nil {
				slog.Error("done finding", "err", err, "n", len(batch))
				return
			}
			if len(batch) == 0 {
				return
			}
			for _, item := range batch {
				lastID = item.GetID()
				if lastID == 0 {
					slog.Error("done finding without lastID", "n", len(batch), "pageSize", pageSize)
					break
				}
				if !yield(item) {
					// cancelled
					return
				}
			}
			if len(batch) < pageSize {
				//slog.Info("done finding", "n", len(batch), "pageSize", pageSize)
				break
			}
		}
	}
}
