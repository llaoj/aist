package session

const summaryMarker = `<compressed_history priority="high">`

// ModelMessages returns the conversation sequence sent to a model: the latest
// compaction summary (if any) followed by raw non-system messages after the
// covered range. Runtime system prompts are assembled dynamically by the caller
// and never persisted as session messages.
func (s *Session) ModelMessages() []Message {
	if s == nil {
		return nil
	}
	raw := conversationMessages(s.Messages)
	latest := s.LatestCompaction()
	if latest == nil {
		return appendMessages(nil, raw)
	}

	out := make([]Message, 0, 1+len(raw))
	out = append(out, summaryMessage(latest))
	for _, message := range raw {
		if message.Seq > latest.CoversToSeq {
			out = append(out, message)
		}
	}
	return out
}

// DisplayMessages returns every conversation message. Legacy system messages
// and synthetic compaction summaries are excluded from the display view.
func (s *Session) DisplayMessages() []Message {
	if s == nil {
		return nil
	}
	return appendMessages(nil, conversationMessages(s.Messages))
}

// RawMessagesAfterCompaction returns conversation messages after the latest
// compaction range. Before the first compaction it returns all conversation messages.
func (s *Session) RawMessagesAfterCompaction() []Message {
	if s == nil {
		return nil
	}
	raw := conversationMessages(s.Messages)
	latest := s.LatestCompaction()
	if latest == nil {
		return appendMessages(nil, raw)
	}
	out := make([]Message, 0, len(raw))
	for _, message := range raw {
		if message.Seq > latest.CoversToSeq {
			out = append(out, message)
		}
	}
	return out
}

func isConversationRole(role Role) bool {
	switch role {
	case RoleUser, RoleAssistant, RoleTool:
		return true
	default:
		return false
	}
}

func conversationMessages(messages []Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		if !isConversationRole(message.Role) {
			continue
		}
		out = append(out, message)
	}
	return out
}

func summaryMessage(compaction *Compaction) Message {
	return Message{
		ID:        "summary_" + compaction.ID,
		Role:      RoleUser,
		Content:   compaction.Summary,
		Synthetic: true,
	}
}

// MessagesAfterSeq returns conversation messages with Seq greater than seq.
func MessagesAfterSeq(s *Session, seq int) []Message {
	if s == nil {
		return nil
	}
	out := make([]Message, 0)
	for _, message := range s.Messages {
		if !isConversationRole(message.Role) {
			continue
		}
		if message.Seq > seq {
			out = append(out, message)
		}
	}
	return out
}

func appendMessages(dst []Message, src []Message) []Message {
	for _, message := range src {
		cloned := message
		cloned.ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
		if message.Usage != nil {
			usage := *message.Usage
			cloned.Usage = &usage
		}
		dst = append(dst, cloned)
	}
	return dst
}

// IsSummary reports whether a message is a synthetic compaction summary.
func IsSummary(message Message) bool {
	return message.Synthetic
}
