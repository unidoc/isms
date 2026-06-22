package mail

import (
	netmail "net/mail"
	"strings"
	"testing"
)

// resolveFrom is the guard against the multi-tenant sender leak (#16): a tenant's
// email must show the tenant's brand in the From display name, never the
// operator's SMTP_FROM display name — while the envelope address (SPF/DKIM)
// stays the configured sender.
func TestResolveFrom(t *testing.T) {
	const addr = "noreply@isms.sh"

	t.Run("no brand keeps SMTP_FROM verbatim", func(t *testing.T) {
		configFrom := `"CommandVector" <` + addr + `>`
		header, envelope := resolveFrom(configFrom, "")
		if header != configFrom {
			t.Errorf("header = %q, want unchanged %q", header, configFrom)
		}
		if envelope != addr {
			t.Errorf("envelope = %q, want bare %q", envelope, addr)
		}
	})

	t.Run("brand overrides the operator display name", func(t *testing.T) {
		// The operator's SMTP_FROM carries their own brand; a tenant must not see it.
		configFrom := `"CommandVector" <` + addr + `>`
		header, envelope := resolveFrom(configFrom, "STS Audit ehf")

		if strings.Contains(header, "CommandVector") {
			t.Fatalf("LEAK: operator display name present in tenant From header: %q", header)
		}
		if envelope != addr {
			t.Errorf("envelope = %q, want bare %q (SPF/DKIM alignment)", envelope, addr)
		}
		// Round-trip: the header must parse back to the tenant brand + the
		// configured envelope address, regardless of quoting details.
		parsed, err := netmail.ParseAddress(header)
		if err != nil {
			t.Fatalf("From header does not parse: %q: %v", header, err)
		}
		if parsed.Name != "STS Audit ehf" {
			t.Errorf("display name = %q, want %q", parsed.Name, "STS Audit ehf")
		}
		if parsed.Address != addr {
			t.Errorf("address = %q, want %q", parsed.Address, addr)
		}
	})

	t.Run("bare SMTP_FROM gains the brand display name", func(t *testing.T) {
		header, envelope := resolveFrom(addr, "Acme Corp")
		if envelope != addr {
			t.Errorf("envelope = %q, want %q", envelope, addr)
		}
		parsed, err := netmail.ParseAddress(header)
		if err != nil {
			t.Fatalf("From header does not parse: %q: %v", header, err)
		}
		if parsed.Name != "Acme Corp" || parsed.Address != addr {
			t.Errorf("parsed = %q <%q>, want %q <%q>", parsed.Name, parsed.Address, "Acme Corp", addr)
		}
	})

	t.Run("non-ASCII brand stays a valid encoded header", func(t *testing.T) {
		header, _ := resolveFrom(addr, "Þórð slf")
		if strings.Contains(header, "Þórð") {
			t.Errorf("non-ASCII name must be RFC 2047 encoded, got raw bytes: %q", header)
		}
		parsed, err := netmail.ParseAddress(header)
		if err != nil {
			t.Fatalf("encoded From header does not parse: %q: %v", header, err)
		}
		if parsed.Name != "Þórð slf" {
			t.Errorf("decoded display name = %q, want %q", parsed.Name, "Þórð slf")
		}
	})
}

// SendBranded with an empty Branding must fall back to the neutral platform name
// ("ISMS"), never the operator's SMTP_FROM display name.
func TestBrandingNameFallback(t *testing.T) {
	if got := (Branding{}).name(); got != "ISMS" {
		t.Errorf("empty Branding.name() = %q, want %q", got, "ISMS")
	}
	if got := (Branding{Name: "STS"}).name(); got != "STS" {
		t.Errorf("Branding.name() = %q, want %q", got, "STS")
	}
}
