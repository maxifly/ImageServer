package promptresolver

import (
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/ydapi"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	runYandexGemmaIntegrationEnv = "RUN_YANDEX_GEMMA_INTEGRATION"
	yandexGemmaTestPromptEnv     = "YANDEX_GEMMA_TEST_PROMPT"
)

type gemmaIntegrationResult struct {
	operationID string
	prompt      string
}

func TestYdPrGemmaIntegration(t *testing.T) {
	if os.Getenv(runYandexGemmaIntegrationEnv) != "1" {
		t.Skipf("set %s=1 to run the real Yandex Gemma integration test", runYandexGemmaIntegrationEnv)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	resolver := NewYdPr(nil, ydapi.NewYdApi(logger), logger)

	sourcePrompt := strings.TrimSpace(os.Getenv(yandexGemmaTestPromptEnv))
	if sourcePrompt == "" {
		sourcePrompt = "Ночной лес, освещённый светлячками, в стиле сказочной иллюстрации"
	}

	first := runGemmaIntegration(t, resolver, sourcePrompt, 1)
	second := runGemmaIntegration(t, resolver, sourcePrompt, 2)

	if first.operationID == second.operationID {
		t.Fatalf("Gemma operation IDs are equal: %q", first.operationID)
	}
	if first.prompt == second.prompt {
		t.Fatalf("Gemma generated identical prompts: %q", first.prompt)
	}

	t.Logf("Source prompt: %s", sourcePrompt)
	t.Logf("First operation %q generated: %s", first.operationID, first.prompt)
	t.Logf("Second operation %q generated: %s", second.operationID, second.prompt)
}

func runGemmaIntegration(t *testing.T, resolver PromptResolver, sourcePrompt string, runNumber int) gemmaIntegrationResult {
	t.Helper()

	result, err := resolver.Resolve(PromptRequest{
		Type: PromptRequestRaw,
		Prompt: &promptmanager.PromptValue{
			Prompt:  sourcePrompt,
			IsGemma: true,
		},
	})
	if err != nil {
		t.Fatalf("run %d: Resolve() error = %v", runNumber, err)
	}
	if !result.IsPending {
		t.Fatalf("run %d: Resolve() returned a non-pending result; cannot compare operation IDs", runNumber)
	}
	if result.OperationID == "" {
		t.Fatalf("run %d: Resolve() returned a pending result without an operation ID", runNumber)
	}

	operationID := result.OperationID
	deadline := time.Now().Add(3 * time.Minute)
	for result.IsPending {
		if time.Now().After(deadline) {
			t.Fatalf("run %d: Gemma operation %q did not complete within 3 minutes", runNumber, operationID)
		}

		t.Logf("run %d: Gemma operation %q is pending", runNumber, operationID)
		time.Sleep(2 * time.Second)

		result, err = resolver.CheckOperation(operationID)
		if err != nil {
			t.Fatalf("run %d: CheckOperation(%q) error = %v", runNumber, operationID, err)
		}
	}

	if result.Prompt == nil {
		t.Fatalf("run %d: Gemma operation completed without a prompt", runNumber)
	}
	if strings.TrimSpace(result.Prompt.Prompt) == "" {
		t.Fatalf("run %d: Gemma operation completed with an empty prompt", runNumber)
	}
	if result.Prompt.IsGemma {
		t.Fatalf("run %d: resolved prompt still has IsGemma=true", runNumber)
	}

	return gemmaIntegrationResult{
		operationID: operationID,
		prompt:      result.Prompt.Prompt,
	}
}
