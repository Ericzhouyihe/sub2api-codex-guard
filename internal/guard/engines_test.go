package guard

import (
	"strconv"
	"strings"
	"testing"
)

func TestExtractCode(t *testing.T) {
	tests := []struct {
		name     string
		answer   string
		language string
		want     string
	}{
		{"html fenced", "说明\n```html\n<h1>hi</h1>\n```\n结束", "html", "<h1>hi</h1>"},
		{"svg fenced", "```SVG\n<svg></svg>\n```", "svg", "<svg></svg>"},
		{"untagged html fallback", "```\n<div>x</div>\n```", "html", "<div>x</div>"},
		{"raw html", "<html><body>x</body></html>", "html", "<html><body>x</body></html>"},
		{"missing", "抱歉，我无法完成", "html", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := extractCode(test.answer, test.language); got != test.want {
				t.Fatalf("extractCode() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFingerprintText(t *testing.T) {
	var numbers []string
	for i := 1; i <= 120; i++ {
		numbers = append(numbers, strconv.Itoa(i))
	}
	text, parsed, err := fingerprintText(strings.Join(numbers, " "), 100)
	if err != nil {
		t.Fatalf("fingerprintText() error = %v", err)
	}
	if parsed != 100 {
		t.Fatalf("parsed = %d, want 100", parsed)
	}
	if len(strings.Split(text, ", ")) != 100 {
		t.Fatalf("text should contain exactly 100 comma-separated numbers")
	}

	if _, _, err := fingerprintText("1 2 3", 100); err == nil {
		t.Fatal("short answer should be rejected")
	}
	if _, _, err := fingerprintText(strings.Repeat("1 ", fingerprintMaxText), 100); err == nil {
		t.Fatal("oversized answer should be rejected")
	}
}

func TestPelicanVariants(t *testing.T) {
	for _, id := range []string{"original", "all-in-one"} {
		variant, ok := findPelicanVariant(id)
		if !ok {
			t.Fatalf("variant %q not found", id)
		}
		if variant.Prompt == "" || variant.language == "" {
			t.Fatalf("variant %q is incomplete", id)
		}
	}
	if _, ok := findPelicanVariant("missing"); ok {
		t.Fatal("unknown variant should not be found")
	}
}

func TestPelicanSVGPage(t *testing.T) {
	variant, _ := findPelicanVariant("all-in-one")
	page, err := variant.page("```svg\n<svg><circle r=\"1\"/></svg>\n```")
	if err != nil {
		t.Fatalf("page() error = %v", err)
	}
	if !strings.HasPrefix(page, "<!doctype html>") || !strings.Contains(page, "<svg>") {
		t.Fatalf("page() = %q", page)
	}
	if _, err := variant.page("没有代码块"); err == nil {
		t.Fatal("missing code block should fail")
	}
}
