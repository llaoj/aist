package session

const summaryMarker = `<compressed_history priority="high">`

// ModelMessages returns the sequence sent to a model: protected system head,
// the latest compaction summary, then raw messages after the covered range.
func (s *Session) ModelMessages() []Message {
	if s == nil {
		return nil
	}
	head, raw := splitHead(s.Messages)
	latest := s.LatestCompaction()
	if latest == nil {
		return appendMessages(nil, s.Messages)
	}

	out := make([]Message, 0, len(head)+1+len(raw))
	out = append(out, head...)
	out = append(out, summaryMessage(latest))
	for _, message := range s.Messages {
		if message.Seq > latest.CoversToSeq {
			out = append(out, message)
		}
	}
	return out
}

// DisplayMessages returns every raw message. Compaction summaries are never
// part of the display view.
func (s *Session) DisplayMessages() []Message {
	if s == nil {
		return nil
	}
	return appendMessages(nil, s.Messages)
}

// HeadMessages returns the protected system prefix.
func (s *Session) HeadMessages() []Message {
	head, _ := splitHead(s.Messages)
	return appendMessages(nil, head)
}

// RawMessagesAfterCompaction returns raw messages after the latest compaction
// range. Before the first compaction it returns all non-head messages.
func (s *Session) RawMessagesAfterCompaction() []Message {
	if s == nil {
		return nil
	}
	_, raw := splitHead(s.Messages)
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

func splitHead(messages []Message) (head []Message, rest []Message) {
	index := 0
	for index < len(messages) && messages[index].Role == RoleSystem {
		index++
	}
	return messages[:index], messages[index:]
}

func summaryMessage(compaction *Compaction) Message {
	return Message{
		ID:        "summary_" + compaction.ID,
		Role:      RoleUser,
		Content:   compaction.Summary,
		Synthetic: true,
	}
}

// MessagesAfterSeq returns raw messages with Seq greater than seq.
func MessagesAfterSeq(s *Session, seq int) []Message {
	if s == nil {
		return nil
	}
	out := make([]Message, 0)
	for _, message := range s.Messages {
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
