package session

// PlanInput describes the current model view split into protected head, prior
// summary and raw messages after the latest compaction.
type PlanInput struct {
	Head             []Message
	Summary          *Message
	PriorSummary     []Message
	RetainTailTokens int
}

// Turn is a user message and the assistant/tool messages that follow it.
type Turn struct {
	Messages []Message
	Start    int
	End      int
}

// Plan is the result of boundary planning.
type Plan struct {
	Head      []Message
	Middle    []Message
	Tail      []Message
	TailStart int
	Feasible  bool
}

// SplitTurns splits raw messages at real user messages. Messages before the
// first user become an orphan prefix; compaction summaries are never turns.
func SplitTurns(raw []Message) (prefix []Message, turns []Turn) {
	current := -1
	for i, message := range raw {
		if message.Role != RoleUser || IsSummary(message) {
			if current >= 0 {
				turns[current].End = i + 1
				turns[current].Messages = append(turns[current].Messages, message)
			} else {
				prefix = append(prefix, message)
			}
			continue
		}
		if current >= 0 {
			turns[current].End = i
		}
		turns = append(turns, Turn{Start: i, End: i + 1})
		current = len(turns) - 1
		turns[current].Messages = append(turns[current].Messages, message)
	}
	if current >= 0 {
		turns[current].End = len(raw)
	}
	return prefix, turns
}

// TailStartTurn returns the index of the first turn kept in the live tail.
func TailStartTurn(turns []Turn, keepTokens int, estimator TokenEstimator) int {
	if estimator == nil {
		estimator = HeuristicEstimator{}
	}
	tailStartTurn := len(turns)
	tailTokens := 0

	for i := len(turns) - 1; i >= 0; i-- {
		turnTokens := estimator.EstimateMessages(turns[i].Messages)

		if tailStartTurn < len(turns) && tailTokens+turnTokens > keepTokens {
			break
		}

		tailStartTurn = i
		tailTokens += turnTokens

		if tailTokens >= keepTokens {
			break
		}
	}

	if tailStartTurn == len(turns) && len(turns) > 0 {
		tailStartTurn = len(turns) - 1
	}
	return tailStartTurn
}

// PlanCompaction computes protected head, compressible middle and live tail.
func PlanCompaction(input PlanInput, estimator TokenEstimator) Plan {
	if estimator == nil {
		estimator = HeuristicEstimator{}
	}
	if len(input.PriorSummary) == 0 {
		return Plan{}
	}

	prefix, turns := SplitTurns(input.PriorSummary)
	if len(turns) == 0 {
		middle := make([]Message, 0, len(prefix)+1)
		if input.Summary != nil {
			middle = append(middle, *input.Summary)
		}
		middle = append(middle, prefix...)
		if len(middle) == 0 {
			return Plan{}
		}
		return Plan{Head: input.Head, Middle: middle, Feasible: true}
	}

	tailStartTurn := TailStartTurn(turns, input.RetainTailTokens, estimator)
	tailStart := turns[tailStartTurn].Start

	middleRaw := input.PriorSummary[:tailStart]
	middle := make([]Message, 0, len(middleRaw)+1)
	if input.Summary != nil {
		middle = append(middle, *input.Summary)
	}
	middle = append(middle, middleRaw...)

	if len(middle) == 0 {
		return Plan{}
	}
	if input.Summary == nil && len(middleRaw) == 0 {
		return Plan{}
	}

	return Plan{
		Head:      input.Head,
		Middle:    middle,
		Tail:      input.PriorSummary[tailStart:],
		TailStart: tailStart,
		Feasible:  true,
	}
}
