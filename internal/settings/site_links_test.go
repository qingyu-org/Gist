package settings

import (
	"strings"
	"testing"
)

func TestParseSiteLinks(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		for _, raw := range []string{"", "  ", "[]", "null"} {
			links, err := parseSiteLinks(raw)
			if err != nil {
				t.Fatalf("parseSiteLinks(%q): %v", raw, err)
			}
			if len(links) != 0 {
				t.Fatalf("parseSiteLinks(%q) = %d links, want none", raw, len(links))
			}
		}
	})

	t.Run("valid", func(t *testing.T) {
		raw := `[{"icon":"home","label":" Homepage ","url":"https://example.com"},{"label":"Blog","url":"/blog"},{"icon":"mail","label":"Mail","url":"mailto:hi@example.com"}]`
		links, err := parseSiteLinks(raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(links) != 3 {
			t.Fatalf("got %d links, want 3", len(links))
		}
		if links[0].Label != "Homepage" || links[0].Icon != "home" {
			t.Errorf("first link not normalised: %#v", links[0])
		}
		if links[1].Icon != "link" {
			t.Errorf("missing icon should fall back to link, got %q", links[1].Icon)
		}
		if links[2].URL != "mailto:hi@example.com" {
			t.Errorf("mailto link changed: %#v", links[2])
		}
	})

	t.Run("unknown icon falls back", func(t *testing.T) {
		links, err := parseSiteLinks(`[{"icon":"nope","label":"X","url":"https://example.com"}]`)
		if err != nil {
			t.Fatal(err)
		}
		if links[0].Icon != "link" {
			t.Errorf("icon = %q, want link", links[0].Icon)
		}
	})

	t.Run("rejects unsafe urls", func(t *testing.T) {
		for _, raw := range []string{
			`[{"label":"X","url":"javascript:alert(1)"}]`,
			`[{"label":"X","url":"//example.com"}]`,
			`[{"label":"X","url":"example.com"}]`,
			`[{"label":"X","url":"https://"}]`,
		} {
			if _, err := parseSiteLinks(raw); err == nil {
				t.Errorf("parseSiteLinks(%q) accepted an unsafe URL", raw)
			}
		}
	})

	t.Run("rejects incomplete and oversized input", func(t *testing.T) {
		if _, err := parseSiteLinks(`[{"label":"","url":"https://example.com"}]`); err == nil {
			t.Error("expected an error for a missing label")
		}
		if _, err := parseSiteLinks(`not json`); err == nil {
			t.Error("expected an error for malformed JSON")
		}
		if _, err := parseSiteLinks(`[{"label":"` + strings.Repeat("a", 41) + `","url":"https://example.com"}]`); err == nil {
			t.Error("expected an error for an oversized label")
		}
		if _, err := parseSiteLinks(`[` + strings.Repeat(`{"label":"X","url":"https://example.com"},`, 12) + `{"label":"X","url":"https://example.com"}]`); err == nil {
			t.Error("expected an error for more than 12 links")
		}
	})

	t.Run("counts unicode labels by characters", func(t *testing.T) {
		label := strings.Repeat("博", 40)
		if _, err := parseSiteLinks(`[{"label":"` + label + `","url":"https://example.com"}]`); err != nil {
			t.Errorf("40-character CJK label should be accepted: %v", err)
		}
	})
}

func TestValidateSiteSettings(t *testing.T) {
	if err := Validate(KeySiteRepository, "https://github.com/qingyu-org/Gist"); err != nil {
		t.Errorf("valid repository URL rejected: %v", err)
	}
	if err := Validate(KeySiteRepository, ""); err != nil {
		t.Errorf("empty repository URL should hide the link: %v", err)
	}
	if err := Validate(KeySiteRepository, "javascript:alert(1)"); err == nil {
		t.Error("javascript URL accepted")
	}
	if err := Validate(KeySiteLinks, `[{"label":"Home","url":"https://example.com","icon":"home"}]`); err != nil {
		t.Errorf("valid links rejected: %v", err)
	}
	if err := Validate(KeySiteLinks, `{`); err == nil {
		t.Error("malformed links accepted")
	}
}
