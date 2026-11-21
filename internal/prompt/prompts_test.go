package prompt

import (
	"strings"
	"testing"
)

type staticMemory string

func (m staticMemory) PromptBlock() string { return string(m) }

func TestBuilderLanguageAndContext(t *testing.T) {
	en := NewBuilder(BuilderOptions{
		Language: "en",
		Memory:   staticMemory("MEMORY_BLOCK"),
	}).Interactive()
	if !strings.Contains(en, "Please respond in English") {
		t.Fatalf("missing English instruction: %s", en)
	}
	if !strings.Contains(en, "MEMORY_BLOCK") {
		t.Fatalf("missing memory block: %s", en)
	}
	if strings.Contains(en, "{{MEMORY}}") || strings.Contains(en, "{{BLACKLIST}}") {
		t.Fatalf("placeholder not replaced: %s", en)
	}
	if strings.Contains(en, "Command Blacklist") {
		t.Fatalf("empty blacklist should not render section: %s", en)
	}

	zh := NewBuilder(BuilderOptions{
		Language: "zh",
		Memory:   staticMemory("MEMORY_BLOCK"),
	}).Interactive()
	if !strings.Contains(zh, "Please respond in Chinese") {
		t.Fatalf("missing Chinese instruction: %s", zh)
	}
}

func TestBuilderMissingContext(t *testing.T) {
	out := NewBuilder(BuilderOptions{Language: "en"}).Interactive()
	if !strings.Contains(out, "no memory files loaded") {
		t.Fatalf("missing memory fallback: %s", out)
	}
	if strings.Contains(out, "{{MEMORY}}") || strings.Contains(out, "{{BLACKLIST}}") {
		t.Fatalf("placeholder not replaced: %s", out)
	}
}

func TestCompactionPrompt(t *testing.T) {
	builder := NewBuilder(BuilderOptions{
		Language:  "en",
		Blacklist: []string{"rm *"},
	})
	if builder.Compaction() == "" {
		t.Fatal("summary prompt is empty")
	}
	if strings.Contains(builder.Compaction(), "Command Blacklist") {
		t.Fatal("compaction prompt must not include blacklist")
	}
}

func TestFollowUpLimitPrompt(t *testing.T) {
	builder := NewBuilder(BuilderOptions{
		Language:  "zh",
		Blacklist: []string{"rm *"},
	})
	out := builder.FollowUpLimit()
	for _, want := range []string{
		"<system-reminder>",
		"MAXIMUM STEPS REACHED",
		"Tools are disabled until the next user input",
		"never claim the task is complete",
		"</system-reminder>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("follow-up limit prompt missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Command Blacklist") {
		t.Fatal("follow-up limit prompt must not include blacklist")
	}
}

func TestToolPolicyCoversWhenAndSafety(t *testing.T) {
	out := NewBuilder(BuilderOptions{
		Language: "en",
		Memory:   staticMemory("MEMORY_BLOCK"),
	}).Interactive()
	for _, want := range []string{
		"Check the Memory catalog before asking",
		"kubectl get nodes",
		"do not scrape known_hosts",
		"Never save secrets",
		"If SSH access fails, report the error and stop",
		"Use file_edit to create or edit regular files",
		"sudo=true",
		"Prompt-based interactive commands",
		"Full-screen TUI programs",
		"MEMORY_BLOCK",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing policy %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		"Prefer append",
		"old_content must be an exact",
		"must confirm",
		"Do not use sudo",
		"never use sudo",
		"Prohibit interactive commands",
		"## Targets and SSH",
		"## File Editing",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("system prompt contains unwanted text %q:\n%s", unwanted, out)
		}
	}
}

func TestInteractivePromptPrefersAutonomyAndConciseness(t *testing.T) {
	out := NewBuilder(BuilderOptions{Language: "en"}).Interactive()
	for _, want := range []string{
		"Work autonomously until the task is complete",
		"stop retrying the same approach",
		"Do not restate the user's request",
		"lead with the outcome",
		"what the user needs to provide",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing guidance %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		"no fixed step limit",
		"Restate question briefly",
		"Summarize the latest tool result",
		"answered or not",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("unexpected verbose guidance %q:\n%s", unwanted, out)
		}
	}
}

func TestPipePromptIsConcise(t *testing.T) {
	out := NewBuilder(BuilderOptions{Language: "en"}).PipeAnalysis()
	if strings.Contains(out, "## Response Example") {
		t.Fatalf("pipe prompt should not embed a full example:\n%s", out)
	}
	if !strings.Contains(out, "analysis only, NO command execution") {
		t.Fatalf("missing pipe mode constraint:\n%s", out)
	}
	if strings.Contains(out, "Command Blacklist") || strings.Contains(out, "{{BLACKLIST}}") {
		t.Fatalf("empty blacklist should not render in pipe prompt:\n%s", out)
	}
}

func TestBuilderBlacklistInjectionAndNormalization(t *testing.T) {
	builder := NewBuilder(BuilderOptions{
		Language: "en",
		Blacklist: []string{
			"  rm   *  ",
			"",
			"   ",
			"rm *",
			"kubectl\ndelete\t*",
			"shutdown",
		},
	})

	for name, out := range map[string]string{
		"interactive": builder.Interactive(),
		"pipe":        builder.PipeAnalysis(),
	} {
		for _, want := range []string{
			"## Command Blacklist",
			"<command_blacklist>",
			"- rm *\n- kubectl delete *\n- shutdown\n",
			"Never execute or suggest any command matching these patterns",
			"Never bypass a blacklisted rule's intent",
			"restricted by the account command blacklist",
		} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s prompt missing %q:\n%s", name, want, out)
			}
		}
		if strings.Contains(out, "{{BLACKLIST}}") {
			t.Fatalf("%s prompt left unreplaced placeholder:\n%s", name, out)
		}
		if count := strings.Count(out, "- rm *\n"); count != 1 {
			t.Fatalf("%s prompt duplicate rule count = %d, want 1", name, count)
		}
	}
}
