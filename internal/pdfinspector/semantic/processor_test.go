package semantic_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"arca/internal/pdfinspector/model"
	"arca/internal/pdfinspector/semantic"
)

func TestProcessExtraction_BasicHierarchy(t *testing.T) {
	proc := semantic.NewProcessor()
	raw := &model.RawExtractionResult{
		Markdown: `# Introduction
This is the intro paragraph.

## Background
Some background details.

### Prior Work
Details on prior work.

## Problem Statement
Problem details.

# Methods
Method overview.`,
	}

	tree, err := proc.ProcessExtraction(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := tree.Validate(); err != nil {
		t.Fatalf("invalid semantic tree: %v", err)
	}

	if len(tree.RootNodes) != 2 {
		t.Fatalf("expected 2 root nodes, got %d", len(tree.RootNodes))
	}

	h1 := tree.RootNodes[0]
	if h1.Heading != "Introduction" || h1.Level != 1 {
		t.Errorf("unexpected root node 0: %+v", h1)
	}
	if len(h1.Children) != 2 {
		t.Fatalf("expected 2 children under Introduction, got %d", len(h1.Children))
	}

	bg := h1.Children[0]
	if bg.Heading != "Background" || bg.Level != 2 {
		t.Errorf("unexpected child 0 of Introduction: %+v", bg)
	}
	if len(bg.Children) != 1 {
		t.Fatalf("expected 1 child under Background, got %d", len(bg.Children))
	}

	pw := bg.Children[0]
	if pw.Heading != "Prior Work" || pw.Level != 3 {
		t.Errorf("unexpected child of Background: %+v", pw)
	}

	ps := h1.Children[1]
	if ps.Heading != "Problem Statement" || ps.Level != 2 {
		t.Errorf("unexpected child 1 of Introduction: %+v", ps)
	}

	h2 := tree.RootNodes[1]
	if h2.Heading != "Methods" || h2.Level != 1 {
		t.Errorf("unexpected root node 1: %+v", h2)
	}
}

func TestProcessExtraction_PageReferences(t *testing.T) {
	proc := semantic.NewProcessor()
	raw := &model.RawExtractionResult{
		Markdown: `# Page One Section
Content on page 1.
<!-- page: 2 -->
More content on page 2.
## Page Two Sub-Section
Sub-content on page 2.
<!-- page 3 -->
Content on page 3 under sub-section.`,
	}

	tree, err := proc.ProcessExtraction(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tree.RootNodes) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(tree.RootNodes))
	}

	root := tree.RootNodes[0]
	if !reflect.DeepEqual(root.PageNumbers, []int{1, 2, 3}) {
		t.Errorf("expected root page numbers [1 2 3], got %v", root.PageNumbers)
	}

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child node, got %d", len(root.Children))
	}

	child := root.Children[0]
	if !reflect.DeepEqual(child.PageNumbers, []int{2, 3}) {
		t.Errorf("expected child page numbers [2 3], got %v", child.PageNumbers)
	}
}

func TestProcessExtraction_DiagnosticWarnings(t *testing.T) {
	proc := semantic.NewProcessor()
	raw := &model.RawExtractionResult{
		Markdown: `# Level 1 Heading
### Level 3 Heading Skipped H2
# 
Valid content after empty heading.`,
	}

	tree, err := proc.ProcessExtraction(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := tree.Validate(); err != nil {
		t.Fatalf("invalid semantic tree: %v", err)
	}

	warnings := proc.Warnings()
	if len(warnings) == 0 {
		t.Errorf("expected diagnostic warnings for skipped level and empty heading, got none")
	}

	hasSkipWarning := false
	hasEmptyHeadingWarning := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "skipped") || strings.Contains(strings.ToLower(w), "jump") || strings.Contains(strings.ToLower(w), "level") {
			hasSkipWarning = true
		}
		if strings.Contains(strings.ToLower(w), "empty") || strings.Contains(strings.ToLower(w), "heading") {
			hasEmptyHeadingWarning = true
		}
	}

	if !hasSkipWarning {
		t.Errorf("expected warning about skipped heading level in %v", warnings)
	}
	if !hasEmptyHeadingWarning {
		t.Errorf("expected warning about empty heading title in %v", warnings)
	}
}

func TestProcessExtraction_JSONLayout(t *testing.T) {
	proc := semantic.NewProcessor()
	raw := &model.RawExtractionResult{
		JSONLayout: map[string]interface{}{
			"nodes": []interface{}{
				map[string]interface{}{
					"type":  "heading",
					"level": 1,
					"text":  "JSON Main Section",
					"page":  1,
				},
				map[string]interface{}{
					"type":  "heading",
					"level": 2,
					"text":  "JSON Sub Section",
					"page":  2,
				},
				map[string]interface{}{
					"type": "unmapped_custom_widget",
					"page": 2,
				},
			},
		},
	}

	tree, err := proc.ProcessExtraction(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tree.RootNodes) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(tree.RootNodes))
	}

	root := tree.RootNodes[0]
	if root.Heading != "JSON Main Section" || root.Level != 1 {
		t.Errorf("unexpected root: %+v", root)
	}

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child node, got %d", len(root.Children))
	}

	warnings := proc.Warnings()
	hasUnmappedWarning := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "unmapped") || strings.Contains(strings.ToLower(w), "custom") {
			hasUnmappedWarning = true
		}
	}
	if !hasUnmappedWarning {
		t.Errorf("expected warning for unmapped node type, got warnings: %v", warnings)
	}
}

func TestProcessExtraction_PreambleContent(t *testing.T) {
	proc := semantic.NewProcessor()
	raw := &model.RawExtractionResult{
		Markdown: `This is document preamble text before any section header.

# Section One
Text under section 1.`,
	}

	tree, err := proc.ProcessExtraction(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := tree.Validate(); err != nil {
		t.Fatalf("invalid semantic tree: %v", err)
	}

	warnings := proc.Warnings()
	hasPreambleWarning := false
	for _, w := range warnings {
		if strings.Contains(strings.ToLower(w), "preamble") || strings.Contains(strings.ToLower(w), "before initial heading") {
			hasPreambleWarning = true
		}
	}
	if !hasPreambleWarning {
		t.Errorf("expected warning for preamble content before first heading, got %v", warnings)
	}
}

func TestProcessExtraction_NilRaw(t *testing.T) {
	proc := semantic.NewProcessor()
	_, err := proc.ProcessExtraction(context.Background(), nil)
	if err == nil {
		t.Errorf("expected error for nil raw extraction result, got nil")
	}
}

func TestProcessExtraction_NodeIDUniqueness(t *testing.T) {
	proc := semantic.NewProcessor()
	raw := &model.RawExtractionResult{
		Markdown: `# Title
## Sub 1
### Sub 1.1
## Sub 2
# Title 2`,
	}

	tree, err := proc.ProcessExtraction(context.Background(), raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ids := make(map[string]bool)
	var checkIDs func(nodes []model.SemanticNode)
	checkIDs = func(nodes []model.SemanticNode) {
		for _, n := range nodes {
			if n.ID == "" {
				t.Errorf("found node with empty ID: %+v", n)
			}
			if ids[n.ID] {
				t.Errorf("found duplicate node ID: %s", n.ID)
			}
			ids[n.ID] = true
			checkIDs(n.Children)
		}
	}
	checkIDs(tree.RootNodes)
}
