package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestAnalyzer_ChannelChecks(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		expectedIssues int
		expectedMsgs   []string
	}{
		{
			name: "unbuffered channel send without select",
			code: `
				package test
				func bad() {
					ch := make(chan int)
					ch <- 1  // should detect this
				}
			`,
			expectedIssues: 2, // One for unbuffered channel, one for send without select
			expectedMsgs: []string{
				"unbuffered channel creation detected",
				"channel send without select statement may block indefinitely",
			},
		},
		{
			name: "buffered channel send without select - should only warn about select",
			code: `
				package test
				func someFunc() {
					ch := make(chan int, 1)
					ch <- 1  // should detect this
				}
			`,
			expectedIssues: 1,
			expectedMsgs: []string{
				"channel send without select statement may block indefinitely",
			},
		},
		{
			name: "proper channel usage - no warnings",
			code: `
				package test
				func good() {
					ch := make(chan int, 1)
					select {
					case ch <- 1:
						// good
					default:
						// also good
					}
				}
			`,
			expectedIssues: 0,
			expectedMsgs:   nil,
		},
		{
			name: "channel in select with default",
			code: `
				package test
				func good() {
					ch := make(chan int)
					select {
					case ch <- 1:
						// good because it's in a select
					default:
						// handles blocking
					}
				}
			`,
			expectedIssues: 1, // Only warns about unbuffered channel
			expectedMsgs: []string{
				"unbuffered channel creation detected",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("failed to parse test code: %v", err)
			}

			analyzer := &Analyzer{
				fset:   fset,
				issues: nil,
				stack:  parentStack{},
			}

			// Check channel declarations
			ast.Inspect(file, func(n ast.Node) bool {
				if n == nil {
					if len(analyzer.stack.nodes) > 0 {
						analyzer.stack.pop()
					}
					return true
				}

				analyzer.stack.push(n)

				switch node := n.(type) {
				case *ast.SendStmt:
					analyzer.checkChannelSend(node)
				case *ast.CallExpr:
					analyzer.checkChannelCreation(node)
				}
				return true
			})

			if got := len(analyzer.issues); got != tt.expectedIssues {
				t.Errorf("got %d issues, want %d", got, tt.expectedIssues)
			}

			// Check that each expected message is present
			for _, expectedMsg := range tt.expectedMsgs {
				found := false
				for _, issue := range analyzer.issues {
					if strings.Contains(issue.Message, expectedMsg) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find message containing %q, but didn't", expectedMsg)
				}
			}
		})
	}
}

// TestAnalyzer_NoFalsePositives tests specific cases that shouldn't trigger warnings
func TestAnalyzer_NoFalsePositives(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "channel send in goroutine with select",
			code: `
				package test
				func good() {
					ch := make(chan int, 1)
					go func() {
						select {
						case ch <- 1:
							// good
						default:
							// good
						}
					}()
				}
			`,
		},
		{
			name: "channel send in nested select",
			code: `
				package test
				func good() {
					ch := make(chan int, 1)
					func() {
						select {
						case ch <- 1:
							select {
							case ch <- 2:
								// good
							default:
								// good
							}
						default:
							// good
						}
					}()
				}
			`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("failed to parse test code: %v", err)
			}

			analyzer := &Analyzer{
				fset:   fset,
				issues: nil,
				stack:  parentStack{},
			}

			ast.Inspect(file, func(n ast.Node) bool {
				if n == nil {
					if len(analyzer.stack.nodes) > 0 {
						analyzer.stack.pop()
					}
					return true
				}

				analyzer.stack.push(n)

				switch node := n.(type) {
				case *ast.SendStmt:
					analyzer.checkChannelSend(node)
				case *ast.CallExpr:
					analyzer.checkChannelCreation(node)
				}
				return true
			})

			if len(analyzer.issues) > 0 {
				t.Errorf("expected no issues, but got %d issues: %v", 
					len(analyzer.issues), 
					formatIssues(analyzer.issues))
			}
		})
	}
}

// Helper function to format issues for error messages
func formatIssues(issues []Issue) string {
	var result strings.Builder
	for i, issue := range issues {
		if i > 0 {
			result.WriteString(", ")
		}
		result.WriteString(fmt.Sprintf("[%s] %s", issue.Severity, issue.Message))
	}
	return result.String()
}
