package session

import (
	"math"
	"unicode"
)

// ContentKind selects the weighting profile for text.
type ContentKind int

const (
	ContentKindNatural ContentKind = iota
	ContentKindStructured
)

const (
	requestOverhead = 32
	messageOverhead = 2
	symbolRatioCut  = 0.25
)

// TokenEstimator estimates token counts deterministically.
type TokenEstimator interface {
	EstimateText(text string, kind ContentKind) int
	EstimateMessage(message Message) int
	EstimateMessages(messages []Message) int
}

// HeuristicEstimator uses character-class weights validated against provider
// usage samples. It intentionally avoids tokenizer dependencies.
type HeuristicEstimator struct{}

// EstimateText estimates one text body.
func (HeuristicEstimator) EstimateText(text string, kind ContentKind) int {
	stats := classifyRunes(text)
	return estimateStats(stats, kind)
}

// EstimateMessage estimates a full message including tool call arguments and
// message framing overhead.
func (e HeuristicEstimator) EstimateMessage(message Message) int {
	kind := messageKind(message)
	total := e.EstimateText(message.Content, kind) + messageOverhead
	for _, call := range message.ToolCalls {
		total += e.EstimateText(call.Function.Name+call.Function.Arguments, ContentKindStructured)
	}
	if message.Name != "" {
		total += e.EstimateText(message.Name, ContentKindNatural)
	}
	return total
}

// EstimateMessages estimates a request, including the fixed request overhead.
func (e HeuristicEstimator) EstimateMessages(messages []Message) int {
	total := 0
	for _, message := range messages {
		total += e.EstimateMessage(message)
	}
	return total + requestOverhead
}

type runeStats struct {
	letters int
	digits  int
	spaces  int
	punct   int
	cjk     int
	other   int
	total   int
}

func classifyRunes(text string) runeStats {
	var stats runeStats
	for _, r := range text {
		stats.total++
		switch {
		case r < unicode.MaxASCII && unicode.IsLetter(r):
			stats.letters++
		case r < unicode.MaxASCII && unicode.IsDigit(r):
			stats.digits++
		case r < unicode.MaxASCII && unicode.IsSpace(r):
			stats.spaces++
		case r < unicode.MaxASCII && unicode.IsPunct(r):
			stats.punct++
		case r >= unicode.MaxASCII && isCJK(r):
			stats.cjk++
		case r >= unicode.MaxASCII && unicode.IsSpace(r):
			stats.spaces++
		case r >= unicode.MaxASCII && (unicode.IsPunct(r) || unicode.IsSymbol(r)):
			stats.punct++
		case r >= unicode.MaxASCII:
			stats.other++
		default:
			stats.other++
		}
	}
	return stats
}

func estimateStats(stats runeStats, kind ContentKind) int {
	var value float64
	if kind == ContentKindStructured {
		value = float64(stats.letters)*0.35 +
			float64(stats.digits)*0.70 +
			float64(stats.spaces)*0.20 +
			float64(stats.punct)*0.60 +
			float64(stats.cjk)*0.80 +
			float64(stats.other)*1.00
	} else {
		value = float64(stats.letters)*0.15 +
			float64(stats.digits)*0.50 +
			float64(stats.spaces)*0.20 +
			float64(stats.punct)*0.50 +
			float64(stats.cjk)*0.70 +
			float64(stats.other)*1.00
	}
	return int(math.Ceil(value))
}

func messageKind(message Message) ContentKind {
	if message.Role == RoleTool {
		return ContentKindStructured
	}
	for _, call := range message.ToolCalls {
		if call.Function.Arguments != "" {
			return ContentKindStructured
		}
	}
	stats := classifyRunes(message.Content)
	if stats.total == 0 {
		return ContentKindNatural
	}
	ratio := float64(stats.digits+stats.punct+stats.spaces) / float64(stats.total)
	if ratio >= symbolRatioCut {
		return ContentKindStructured
	}
	return ContentKindNatural
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

// EstimateForTrigger implements the "last provider total + new-message delta"
// estimation model.
func EstimateForTrigger(s *Session, estimator TokenEstimator) int {
	if s == nil {
		return 0
	}
	if estimator == nil {
		estimator = HeuristicEstimator{}
	}
	if !s.TokenAnchor.Valid {
		return estimator.EstimateMessages(s.ModelMessages())
	}
	delta := estimator.EstimateMessages(MessagesAfterSeq(s, s.TokenAnchor.AfterSeq))
	return s.TokenAnchor.TotalTokens + delta
}
