package store

import "context"

func (s *Store) PinnedMessages(ctx context.Context, conv int64) ([]Message, error) {
	return s.messages(ctx, `m.conversation_id=$1 AND m.id IN (SELECT message_id FROM conversation_pins WHERE conversation_id=$1)`, conv, 0, 1)
}
