package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Position represents a range in the source code
type Position struct {
	Filename     string `json:"filename"`
	StartLine    int    `json:"start_line"`
	StartColumn  int    `json:"start_column"`
	EndLine      int    `json:"end_line"`
	EndColumn    int    `json:"end_column"`
}

func (p Position) String() string {
	if p.StartLine == p.EndLine {
		return fmt.Sprintf("%s:%d:%d-%d", p.Filename, p.StartLine, p.StartColumn, p.EndColumn)
	}
	return fmt.Sprintf("%s:%d:%d-%d:%d", p.Filename, p.StartLine, p.StartColumn, p.EndLine, p.EndColumn)
}

type Issue struct {
	Pos      Position
	Message  string
	Severity string
}

type Analyzer struct {
	issues []Issue
	fset   *token.FileSet
	stack  parentStack
}

// getPosition converts ast node position information into a Position
func (a *Analyzer) getPosition(start, end token.Pos) Position {
	startPos := a.fset.Position(start)
	endPos := a.fset.Position(end)
	
	return Position{
		Filename:     startPos.Filename,
		StartLine:    startPos.Line,
		StartColumn:  startPos.Column,
		EndLine:      endPos.Line,
		EndColumn:    endPos.Column,
	}
}

type parentStack struct {
	nodes []ast.Node
}

func (p *parentStack) push(n ast.Node) {
	if n == nil {
		return
	}
	p.nodes = append(p.nodes, n)
}

func (p *parentStack) pop() {
	if len(p.nodes) > 0 {
		p.nodes = p.nodes[:len(p.nodes)-1]
	}
}

type OutputFormat string

const (
	OutputFormatText OutputFormat = "txt"
	OutputFormatJSON OutputFormat = "json"
)

type JSONOutput struct {
	Issues []JSONIssue `json:"issues"`
	Total  int         `json:"total"`
}

type JSONIssue struct {
	Severity    string   `json:"severity"`
	Message     string   `json:"message"`
	Position    Position `json:"position"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	path := flag.String("path", ".", "Path to file or directory to analyze")
	output := flag.String("output", "txt", "Output format: txt or json")
	flag.Parse()

	if path == nil || output == nil {
		return fmt.Errorf("invalid flag values")
	}

	outputFormat := OutputFormat(*output)
	if outputFormat != OutputFormatText && outputFormat != OutputFormatJSON {
		return fmt.Errorf("invalid output format: %s. Valid options are: txt, json", *output)
	}

	analyzer := &Analyzer{
		fset: token.NewFileSet(),
	}
	if analyzer.fset == nil {
		return fmt.Errorf("failed to create token.FileSet")
	}

	if err := analyzer.analyzePath(*path); err != nil {
		return fmt.Errorf("error analyzing path: %w", err)
	}

	if err := printOutput(outputFormat, analyzer.issues); err != nil {
		return fmt.Errorf("error printing output: %w", err)
	}

	return nil
}

func printOutput(format OutputFormat, issues []Issue) error {
	switch format {
	case OutputFormatJSON:
		return printJSON(issues)
	case OutputFormatText:
		return printText(issues)
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}

func printJSON(issues []Issue) error {
	output := JSONOutput{
		Total:  len(issues),
		Issues: make([]JSONIssue, len(issues)),
	}

	for i, issue := range issues {
		output.Issues[i] = JSONIssue{
			Severity: issue.Severity,
			Message:  issue.Message,
			Position: issue.Pos,
		}
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %w", err)
	}
	
	_, err = fmt.Println(string(jsonBytes))
	return err
}

func printText(issues []Issue) error {
	if len(issues) == 0 {
		_, err := fmt.Println("No issues found!")
		return err
	}

	_, err := fmt.Printf("Found %d potential issues:\n\n", len(issues))
	if err != nil {
		return err
	}

	for _, issue := range issues {
		if _, err := fmt.Printf("[%s] %s: %s\n", issue.Severity, issue.Pos, issue.Message); err != nil {
			return err
		}
	}
	return nil
}

func (a *Analyzer) analyzePath(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error accessing path: %w", err)
	}

	if fileInfo.IsDir() {
		return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".go") {
				if err := a.analyzeFile(path); err != nil {
					return fmt.Errorf("error analyzing file %s: %w", path, err)
				}
			}
			return nil
		})
	}

	return a.analyzeFile(path)
}

func (a *Analyzer) analyzeFile(path string) error {
	file, err := parser.ParseFile(a.fset, path, nil, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("error parsing file: %w", err)
	}

	if file == nil {
		return fmt.Errorf("parsed file is nil")
	}

	a.analyze(file)
	return nil
}

func (a *Analyzer) analyze(file *ast.File) {
	if file == nil {
		return
	}

	// Reset the stack for each file
	a.stack = parentStack{}

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			if len(a.stack.nodes) > 0 {
				a.stack.pop()
			}
			return true
		}

		a.stack.push(n)

		switch node := n.(type) {
		case *ast.SendStmt:
			if node != nil {
				a.checkChannelSend(node)
			}
		case *ast.CallExpr:
			if node != nil {
				a.checkChannelCreation(node)
			}
		}
		return true
	})
}

func (a *Analyzer) checkChannelSend(node *ast.SendStmt) {
	// Check if any parent is a select statement
	inSelect := false
	for _, parent := range a.stack.nodes {
		if parent == nil {
			continue
		}
		if _, ok := parent.(*ast.SelectStmt); ok {
			inSelect = true
			break
		}
	}

	if !inSelect {
		a.addIssue(Issue{
			Pos:      a.getPosition(node.Pos(), node.End()),
			Message:  "channel send without select statement may block indefinitely",
			Severity: "WARNING",
		})
	}
}

func (a *Analyzer) checkChannelCreation(node *ast.CallExpr) {
	fun, ok := node.Fun.(*ast.Ident)
	if !ok || fun == nil || fun.Name != "make" {
		return
	}

	if len(node.Args) > 0 {
		if chanType, ok := node.Args[0].(*ast.ChanType); ok && chanType != nil {
			// Check if buffer size is specified
			if len(node.Args) == 1 {
				a.addIssue(Issue{
					Pos:      a.getPosition(node.Pos(), node.End()),
					Message:  "unbuffered channel creation detected - consider specifying buffer size",
					Severity: "INFO",
				})
			}
		}
	}
}

func (a *Analyzer) addIssue(issue Issue) {
	a.issues = append(a.issues, issue)
}
