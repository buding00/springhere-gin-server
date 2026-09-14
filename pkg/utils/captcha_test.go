package utils

import (
	"strings"
	"testing"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/google/uuid"
)

func TestImageCaptchaGenerate(t *testing.T) {
	generator := NewImageCaptcha(config.CaptchaConfig{Length: 4, Width: 160, Height: 48})
	id, image, answer, err := generator.Generate()
	if err != nil {
		t.Fatalf("generate captcha: %v", err)
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("captcha id is not a uuid: %v", err)
	}
	if !strings.HasPrefix(image, "data:image/png;base64,") {
		t.Fatalf("unexpected image data url prefix: %q", image[:min(len(image), 32)])
	}
	if len(answer) != 4 {
		t.Fatalf("expected 4-character answer, got %q", answer)
	}
	if answer != strings.ToUpper(answer) {
		t.Fatalf("expected uppercase answer, got %q", answer)
	}
}
