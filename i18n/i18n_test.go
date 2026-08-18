package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestTfNetworkErrorFormatsWrappedError(t *testing.T) {
	SetLang(LangZH)
	message := Tf("tf_network_error", errors.New("terraform init failed"))
	if strings.Contains(message, "%!") {
		t.Fatalf("formatted network error contains fmt diagnostic: %q", message)
	}
	if !strings.Contains(message, "terraform init failed") {
		t.Fatalf("formatted network error = %q, want original error", message)
	}
}
