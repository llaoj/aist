package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/fatih/color"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

// markdownParser parses the GFM subset we render: tables and strikethrough
// (the latter is shown as plain text).
var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough),
).Parser()

// Markdown styles. fatih/color disables colour automatically when stdout is
// not a terminal, so the structural layout stays usable when redirected.
var (
	mdHeading = color.New(color.Bold, color.FgHiCyan)
	mdBold    = color.New(color.Bold)
	mdItalic  = color.New(color.Italic)
	mdCode    = color.New(color.FgHiYellow)
	mdDim     = color.New(color.Faint)
	mdQuote   = color.New(color.FgHiBlack)
)

// RenderMarkdown renders Markdown into styled terminal output using goldmark's
// AST: headings, lists, blockquotes, tables and fenced/indented code blocks.
func RenderMarkdown(source string) string {
	src := []byte(source)
	r := &mdRenderer{src: src}
	r.blocks(markdownParser.Parse(gmtext.NewReader(src)), 0)
	return strings.TrimRight(r.b.String(), "\n")
}

// mdRenderer walks a goldmark AST and writes terminal text. Nested content
// (list items, blockquotes) is rendered by a fresh child renderer and then
// re-prefixed, which keeps markers and indentation correct at any depth.
type mdRenderer struct {
	src []byte
	b   strings.Builder
}

func (r *mdRenderer) write(s string) { r.b.WriteString(s) }

// blocks renders the block-level children of n at the given indentation.
func (r *mdRenderer) blocks(n ast.Node, indent int) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.block(c, indent)
	}
}

func (r *mdRenderer) block(n ast.Node, indent int) {
	pad := indentString(indent)

	switch node := n.(type) {
	case *ast.Heading:
		r.write(pad + mdHeading.Sprint(r.inline(node)) + "\n\n")
	case *ast.Paragraph, *ast.TextBlock:
		r.write(pad + strings.TrimRight(r.inline(node), "\n") + "\n\n")
	case *ast.FencedCodeBlock:
		r.codeBlock(node.Lines(), indent)
	case *ast.CodeBlock:
		r.codeBlock(node.Lines(), indent)
	case *ast.ThematicBreak:
		r.write(pad + mdDim.Sprint(strings.Repeat("─", 40)) + "\n\n")
	case *ast.Blockquote:
		r.write(prefixLines(r.render(node), pad+mdQuote.Sprint("│ ")) + "\n")
	case *ast.List:
		r.list(node, indent)
	case *extast.Table:
		r.table(node, indent)
	default:
		// Unknown/inline-only blocks (e.g. raw HTML) render via their children.
		r.blocks(node, indent)
	}
}

// render returns the child blocks of n rendered at the root indentation.
func (r *mdRenderer) render(n ast.Node) string {
	inner := &mdRenderer{src: r.src}
	inner.blocks(n, 0)
	return strings.TrimRight(inner.b.String(), "\n")
}

// prefixLines prepends prefix to every line of s, preserving the trailing
// newline so callers can add blank-line spacing themselves.
func prefixLines(s, prefix string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

// codeBlock renders fenced/indented code with a small indent. Syntax
// highlighting is intentionally omitted to keep the binary small.
func (r *mdRenderer) codeBlock(lines *gmtext.Segments, indent int) {
	pad := indentString(indent + 1)
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		r.write(pad + mdCode.Sprint(strings.TrimRight(string(seg.Value(r.src)), "\n")) + "\n")
	}
	r.write("\n")
}

func (r *mdRenderer) list(node *ast.List, indent int) {
	pad := indentString(indent)
	index := node.Start
	if index == 0 {
		index = 1
	}

	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "- "
		if node.IsOrdered() {
			marker = fmt.Sprintf("%d. ", index)
			index++
		}

		inner := &mdRenderer{src: r.src}
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			switch c.(type) {
			case *ast.TextBlock, *ast.Paragraph:
				inner.write(strings.TrimRight(inner.inline(c), "\n") + "\n")
			default:
				inner.block(c, 0)
			}
		}

		lines := strings.Split(strings.TrimRight(inner.b.String(), "\n"), "\n")
		r.write(pad + marker + lines[0] + "\n")
		for _, line := range lines[1:] {
			r.write(pad + strings.Repeat(" ", len(marker)) + line + "\n")
		}
	}
	r.write("\n")
}

// inline renders the inline children of n into a string.
func (r *mdRenderer) inline(n ast.Node) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		b.WriteString(r.inlineNode(c))
	}
	return b.String()
}

func (r *mdRenderer) inlineNode(n ast.Node) string {
	switch node := n.(type) {
	case *ast.Text:
		s := string(node.Segment.Value(r.src))
		if node.SoftLineBreak() || node.HardLineBreak() {
			s += "\n"
		}
		return s
	case *ast.CodeSpan:
		return mdCode.Sprint(r.inline(node))
	case *ast.Emphasis:
		if node.Level == 2 {
			return mdBold.Sprint(r.inline(node))
		}
		return mdItalic.Sprint(r.inline(node))
	case *ast.Link:
		return fmt.Sprintf("%s (%s)", r.inline(node), node.Destination)
	case *ast.AutoLink:
		return string(node.Label(r.src))
	default:
		return r.inline(node)
	}
}

// table renders a box-drawing grid with left-aligned cells.
func (r *mdRenderer) table(t *extast.Table, indent int) {
	var header []string
	var rows [][]string
	for n := t.FirstChild(); n != nil; n = n.NextSibling() {
		switch row := n.(type) {
		case *extast.TableHeader:
			header = r.cells(row)
		case *extast.TableRow:
			rows = append(rows, r.cells(row))
		}
	}

	cols := len(header)
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return
	}

	widths := make([]int, cols)
	measure := func(row []string) {
		for i, cell := range row {
			if w := ansi.StringWidth(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}
	measure(header)
	for _, row := range rows {
		measure(row)
	}

	pad := indentString(indent)
	dataRow := func(row []string, isHeader bool) string {
		cells := make([]string, len(widths))
		for i, w := range widths {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			if isHeader {
				cell = mdBold.Sprint(cell)
			}
			cells[i] = " " + padRight(cell, w) + " "
		}
		return pad + "│" + strings.Join(cells, "│") + "│\n"
	}
	border := func(left, mid, right string) string {
		cells := make([]string, len(widths))
		for i, w := range widths {
			cells[i] = strings.Repeat("─", w+2)
		}
		return pad + left + strings.Join(cells, mid) + right + "\n"
	}

	r.write(border("┌", "┬", "┐"))
	r.write(dataRow(header, true))
	r.write(border("├", "┼", "┤"))
	for _, row := range rows {
		r.write(dataRow(row, false))
	}
	r.write(border("└", "┴", "┘") + "\n")
}

func (r *mdRenderer) cells(row ast.Node) []string {
	var out []string
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		if cell, ok := c.(*extast.TableCell); ok {
			out = append(out, strings.TrimSpace(r.inline(cell)))
		}
	}
	return out
}

func indentString(n int) string { return strings.Repeat("  ", n) }

// padRight appends spaces so s occupies exactly w terminal columns, ignoring
// any ANSI escapes already present in s.
func padRight(s string, w int) string { return s + strings.Repeat(" ", w-ansi.StringWidth(s)) }
