package compattest

import (
	"testing"
)

func TestRunAgent(t *testing.T) {
	result, err := RunAgent(t.Context())
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if result.Text != "hello" {
		t.Fatalf("RunAgent text = %q, want hello", result.Text)
	}
}

func TestPublicFacade(t *testing.T) {
	messageID, typedRaw, err := NativeCompletion(t.Context())
	if err != nil || messageID != "msg_external" || !typedRaw {
		t.Fatalf("NativeCompletion = %q, %t, %v", messageID, typedRaw, err)
	}

	embedding, err := Embed(t.Context())
	if err != nil || len(embedding) != 3 {
		t.Fatalf("Embed = %v, %v", embedding, err)
	}

	language, image, err := RegistryModels()
	if err != nil || language.ModelID() != "gemini-language" || image.ModelID() != "gemini-image" {
		t.Fatalf("RegistryModels = %#v, %#v, %v", language, image, err)
	}
}
