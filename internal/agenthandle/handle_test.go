package agenthandle

import (
	"strings"
	"testing"
)

func TestHandlePolicy(t *testing.T) {
	for _, value := range []string{"GROKBOT", "Grok.Bot_7-X", "muse", "42", "_A", ".-", strings.Repeat("A", MaxLength)} {
		if !Valid(value) {
			t.Errorf("documented handle rejected: %q", value)
		}
	}
	for _, value := range []string{"", "A", "Grok Bot", "Grok/Bot", "Grok@Bot", "Grok:Bot", "<Grok>", "Grok\nBot", "Grok\x00Bot", "Ågent", "Ｇrok", strings.Repeat("A", MaxLength+1)} {
		if Valid(value) {
			t.Errorf("invalid handle accepted: %q", value)
		}
	}
}
