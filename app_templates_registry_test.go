package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteLocalReadmeReferences_InlinesImagesAndRewritesLinks(t *testing.T) {
	tmpDir := t.TempDir()
	imageDir := filepath.Join(tmpDir, "img")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("mkdir img dir: %v", err)
	}

	imagePath := filepath.Join(imageDir, "demo.png")
	if err := os.WriteFile(imagePath, []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	docPath := filepath.Join(tmpDir, "docs.md")
	if err := os.WriteFile(docPath, []byte("# docs"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	content := "![demo](./img/demo.png)\n[details](./docs.md#usage)\n[remote](https://example.com)"
	rewritten := rewriteLocalReadmeReferences(content, []string{tmpDir})

	if !strings.Contains(rewritten, "data:image/png;base64,") {
		t.Fatalf("expected image to be inlined, got %q", rewritten)
	}

	if !strings.Contains(rewritten, "file://") || !strings.Contains(rewritten, "#usage") {
		t.Fatalf("expected relative link to become file URL with fragment, got %q", rewritten)
	}

	if !strings.Contains(rewritten, "https://example.com") {
		t.Fatalf("expected absolute link to remain untouched, got %q", rewritten)
	}
}

func TestResolveLocalReadmePath_UsesFallbackAssetDirs(t *testing.T) {
	firstDir := filepath.Join(t.TempDir(), "case")
	secondDir := filepath.Join(t.TempDir(), "template")
	if err := os.MkdirAll(filepath.Join(firstDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir first assets: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(secondDir, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir second assets: %v", err)
	}

	expected := filepath.Join(secondDir, "assets", "guide.md")
	if err := os.WriteFile(expected, []byte("guide"), 0o644); err != nil {
		t.Fatalf("write fallback file: %v", err)
	}

	resolved, err := resolveLocalReadmePath("./assets/guide.md", []string{firstDir, secondDir})
	if err != nil {
		t.Fatalf("resolve fallback path: %v", err)
	}

	if resolved != expected {
		t.Fatalf("resolved path = %s, want %s", resolved, expected)
	}
}