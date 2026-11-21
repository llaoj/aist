package markdown

import (
	"errors"
	"strings"
	"time"

	"github.com/llaoj/aist/internal/memory"
	"gopkg.in/yaml.v3"
)

type frontmatter struct {
	Updated string `yaml:"updated,omitempty"`
	Source  string `yaml:"source,omitempty"`
	Summary string `yaml:"summary,omitempty"`
	Type    string `yaml:"type,omitempty"`
}

// Encode encodes a Memory using YAML frontmatter for metadata.
func Encode(mem memory.Entry) ([]byte, error) {
	name := strings.TrimSpace(mem.Name)
	if name == "" {
		return nil, errors.New("memory name is empty")
	}

	var sb strings.Builder
	meta := frontmatter{
		Source:  strings.TrimSpace(mem.Source),
		Summary: strings.TrimSpace(mem.Summary),
		Type:    strings.TrimSpace(mem.Type),
	}
	if !mem.Updated.IsZero() {
		meta.Updated = mem.Updated.Format(time.RFC3339)
	}
	if meta != (frontmatter{}) {
		data, err := yaml.Marshal(meta)
		if err != nil {
			return nil, err
		}
		sb.WriteString("---\n")
		sb.Write(data)
		sb.WriteString("---\n\n")
	}

	sb.WriteString("# ")
	sb.WriteString(name)
	sb.WriteString("\n")

	sb.WriteString("\n## Memory\n")
	if content := strings.TrimSpace(mem.Content); content != "" {
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}
	return []byte(sb.String()), nil
}
