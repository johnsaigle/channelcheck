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

type Issue struct {
	Pos      token.Position
	Message  string
	Severity string
}

type Analyzer struct {
	issues []Issue
	fset   *token.FileSet
	stack  parentStack
}

type parentStack struct {
	nodes []ast.Node
}

func (p *parentStack) push(n ast.Node) {
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
    Severity  string `json:"severity"`
    Message   string `json:"message"`
    File      string `json:"file"`
    Line      int    `json:"line"`
    Column    int    `json:"column"`
    Position  string `json:"position"`
}

func main() {
    path := flag.String("path", ".", "Path to file or directory to analyze")
    output := flag.String("output", "txt", "Output format: txt or json")
    flag.Parse()

    outputFormat := OutputFormat(*output)
    if outputFormat != OutputFormatText && outputFormat != OutputFormatJSON {
        log.Fatalf("Invalid output format: %s. Valid options are: txt, json", *output)
    }

    analyzer := &Analyzer{
        fset: token.NewFileSet(),
    }

    err := analyzer.analyzePath(*path)
    if err != nil {
        log.Printf("Error analyzing path: %v\n", err)
    }

    switch outputFormat {
    case OutputFormatJSON:
        printJSON(analyzer.issues)
    case OutputFormatText:
        printText(analyzer.issues)
    }
}

func printJSON(issues []Issue) {
    output := JSONOutput{
        Total: len(issues),
        Issues: make([]JSONIssue, len(issues)),
    }

    for i, issue := range issues {
        output.Issues[i] = JSONIssue{
            Severity: issue.Severity,
            Message:  issue.Message,
            File:     issue.Pos.Filename,
            Line:     issue.Pos.Line,
            Column:   issue.Pos.Column,
            Position: issue.Pos.String(),
        }
    }

    jsonBytes, err := json.MarshalIndent(output, "", "  ")
    if err != nil {
        log.Fatalf("Error marshaling JSON: %v", err)
    }
    fmt.Println(string(jsonBytes))
}

func printText(issues []Issue) {
    if len(issues) == 0 {
        fmt.Println("No issues found!")
        return
    }

    fmt.Printf("Found %d potential issues:\n\n", len(issues))
    for _, issue := range issues {
        fmt.Printf("[%s] %s: %s\n", issue.Severity, issue.Pos, issue.Message)
    }
}

func (a *Analyzer) analyzePath(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("error accessing path: %v", err)
	}

	if fileInfo.IsDir() {
		return filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".go") {
				return a.analyzeFile(path)
			}
			return nil
		})
	}

	return a.analyzeFile(path)
}


func (a *Analyzer) analyzeFile(path string) error {
	// Parse the file
	file, err := parser.ParseFile(a.fset, path, nil, parser.AllErrors)
	if err != nil {
		return fmt.Errorf("error parsing file %s: %v", path, err)
	}
	a.analyze(file)
	return nil
}

func (a *Analyzer) analyze(file *ast.File) {
	// Reset the stack for each file
	a.stack = parentStack{}

	// Check channel declarations
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
			// Check for channel sends
			a.checkChannelSend(node)
		case *ast.CallExpr:
			// Check for make(chan) calls
			a.checkChannelCreation(node)
		}
		return true
	})
}

func (a *Analyzer) checkChannelSend(node *ast.SendStmt) {
	// Check if any parent is a select statement
	inSelect := false
	for _, parent := range a.stack.nodes {
		if _, ok := parent.(*ast.SelectStmt); ok {
			inSelect = true
			break
		}
	}

	if !inSelect {
		a.addIssue(Issue{
			Pos:      a.fset.Position(node.Pos()),
			Message:  "channel send without select statement may block indefinitely",
			Severity: "WARNING",
		})
	}
}

func (a *Analyzer) checkChannelCreation(node *ast.CallExpr) {
	// Check if it's a make(chan) call
	if fun, ok := node.Fun.(*ast.Ident); ok && fun.Name == "make" {
		if len(node.Args) > 0 {
			if _, ok := node.Args[0].(*ast.ChanType); ok {
				// Check if buffer size is specified
				if len(node.Args) == 1 {
					a.addIssue(Issue{
						Pos:      a.fset.Position(node.Pos()),
						Message:  "unbuffered channel creation detected - consider specifying buffer size",
						Severity: "INFO",
					})
				}
			}
		}
	}
}

func (a *Analyzer) addIssue(issue Issue) {
	a.issues = append(a.issues, issue)
}
